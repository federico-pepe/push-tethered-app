package midi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/federico-pepe/push-tethered-app/internal/midilock"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
	gm "gitlab.com/gomidi/midi/v2"
)

// PortRef identifies one Push MIDI cable and the physical unit it belongs to.
//
// InNum/OutNum are the driver's own port numbers (drivers.Port.Number()) and
// are the only way to open a specific cable when two units present identical
// names — gomidi's name lookup does a substring match and returns whichever
// port it finds first (drivers/port.go, InByName/OutByName), so two Pushes
// named the same way are otherwise indistinguishable by name alone.
type PortRef struct {
	InName  string         `json:"inName"`
	OutName string         `json:"outName"` // "" when no matching out cable was found
	InNum   int            `json:"inNum"`
	OutNum  int            `json:"outNum"` // -1 when OutName == ""
	Unit    string         `json:"unit"`
	Cable   int            `json:"cable"` // 1-based index within the unit, in the device's own cable order
	Role    string         `json:"role"`  // "Live" | "User" | "External" | "" (unknown — WinMM has no jack strings)
	Device  pushmap.Device `json:"device"`
	IsLive  bool           `json:"isLive"`

	// Ambiguous is set when two or more units produced indistinguishable
	// names and this ref could not be safely paired or grouped. OutNum is -1
	// whenever this is true: the caller must let the user pick the out cable
	// explicitly, identified by flashing its LEDs, rather than guess.
	Ambiguous bool `json:"ambiguous"`
}

// Unit groups every cable that belongs to one physical Push.
type Unit struct {
	Key    string         `json:"key"`
	Device pushmap.Device `json:"device"`
	Ports  []PortRef      `json:"ports"`
}

// roleOrder gives each named role a stable cable position, matching the order
// Push itself exposes them: Live first, then User, then External (Push 3
// only). See docs/protocol/midi-input.md.
var roleOrder = map[string]int{"Live": 1, "User": 2, "External": 3}

var roleSuffixes = []string{"Live Port", "User Port", "External Port"}

// winmmWrapped matches WinMM's naming for every cable after the first, e.g.
// "MIDIIN2 (Ableton Push 3 MIDI)" or "MIDIOUT3 (Ableton Push 3 MIDI)". WinMM
// exposes no jack strings at all, so the wrapped device name is the only unit
// identity available — see docs/platform/windows.md.
var winmmWrapped = regexp.MustCompile(`^MIDI(?:IN|OUT)(\d+) \((.+)\)$`)

// alsaClientPort strips the trailing "<client>:<port>" ALSA sequencer
// address rtmididrv appends to every port name on Linux, e.g.
// "Ableton Push 2:Ableton Push 2 Live Port 28:0" — confirmed live on real
// Raspberry Pi 5 hardware 2026-08-19. Without stripping it, the name no
// longer ends in "Live Port"/"User Port", so the role-suffix check below
// missed every cable: both Live and User fell through to the unnamed-cable-1
// default, making each its own fake single-cable "unit" and both wrongly
// marked Live.
var alsaClientPort = regexp.MustCompile(`\s+\d+:\d+$`)

// winmmIndex strips a trailing " <n>" that this driver's Windows backend
// appends to every MIDI port name, not just Push's — confirmed live on real
// Windows hardware 2026-08-19, where the number matched each port's own
// PortRef.InNum/OutNum exactly ("Ableton Push 3 MIDI 0",
// "MIDIIN2 (Ableton Push 3 MIDI) 1", "Ableton Push 2 0", the index
// incrementing globally across every Push-named port the OS reports, not
// reset per unit). Without stripping it: the bare cable-1 name no longer
// matches the "no role suffix, cable 1" default cleanly against a
// same-shaped output name (the two sides get different numbers, since
// Windows enumerates MIDI in and out independently), and the wrapped form
// ("MIDIIN2 (...) 1") no longer matches winmmWrapped at all, since that
// regex anchors to the closing paren. This is why every cable showed
// "unknown role, Live" and "no output cable found" — role detection and
// wrapped-cable detection both silently missed on every single port.
//
// Safe on every other platform: CoreMIDI/ALSA names never end in a bare
// space-then-digits — ALSA's own decoration ends in "<client>:<port>", which
// this pattern's required whitespace-before-digit does not match a colon
// against, and alsaClientPort already strips that shape first regardless.
var winmmIndex = regexp.MustCompile(`\s+\d+$`)

// unitKeyOf splits a port name into the unit it belongs to, the cable's named
// role if it has one, and its 1-based position among that unit's cables.
//
// Three shapes are handled, in order: CoreMIDI/ALSA role-suffixed names
// (which carry the true role — platform-specific decoration is stripped
// first, see alsaClientPort and winmmIndex), WinMM's wrapped names for cable
// 2 and up, and WinMM's bare device name for cable 1. Anything else is
// treated as a single unnamed-role cable 1, which keeps today's single-unit
// behaviour rather than failing outright on a naming convention this code
// has not seen yet.
func unitKeyOf(name string) (unit, role string, cable int) {
	trimmed := alsaClientPort.ReplaceAllString(name, "")
	trimmed = winmmIndex.ReplaceAllString(trimmed, "")
	for _, suffix := range roleSuffixes {
		if strings.HasSuffix(trimmed, suffix) {
			unit = strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
			role = strings.TrimSuffix(suffix, " Port")
			return unit, role, roleOrder[role]
		}
	}

	if m := winmmWrapped.FindStringSubmatch(trimmed); m != nil {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		return m[2], "", n
	}

	return trimmed, "", 1
}

// portName pairs a driver-reported name with its stable port number.
type portName struct {
	Name string
	Num  int
}

// groupPorts pairs input and output cables into per-unit PortRefs. It is pure
// over the two name lists so the platform naming cases — and the still-
// unmeasured two-identical-units cases — are table-testable without hardware.
func groupPorts(ins, outs []portName) []PortRef {
	type cableKey struct {
		unit  string
		cable int
	}

	// First pass: does any (unit, cable) key appear more than once on either
	// side? That means two physical units produced indistinguishable names —
	// the case neither CoreMIDI, ALSA, nor WinMM has been confirmed to avoid
	// with two identical Push units attached (see plans/2026-08-19-multi-device.md).
	// Grouping such a key would silently pick one unit's cable for both, so
	// every ref sharing an ambiguous key is surfaced unpaired instead.
	ambiguousKeys := map[cableKey]bool{}
	seenIn := map[cableKey]bool{}
	for _, in := range ins {
		unit, _, cable := unitKeyOf(in.Name)
		k := cableKey{unit, cable}
		if seenIn[k] {
			ambiguousKeys[k] = true
		}
		seenIn[k] = true
	}
	seenOut := map[cableKey]bool{}
	for _, out := range outs {
		unit, _, cable := unitKeyOf(out.Name)
		k := cableKey{unit, cable}
		if seenOut[k] {
			ambiguousKeys[k] = true
		}
		seenOut[k] = true
	}

	// A unit with any ambiguous cable never gets the positional fallback
	// below, even on its non-ambiguous cables: outsByUnit pools every output
	// sharing that unit's name string, which for two indistinguishable units
	// means pooling cables from two different physical boxes. Cross-pairing
	// one unit's input with the other unit's output there would be worse
	// than leaving it unpaired.
	ambiguousUnits := map[string]bool{}
	for k := range ambiguousKeys {
		ambiguousUnits[k.unit] = true
	}

	outByExactName := map[string]portName{}
	for _, out := range outs {
		outByExactName[out.Name] = out
	}

	// outsByUnit groups outputs by unit NAME only (not the (unit, cable) key
	// exact matching uses), ordered by each output's own embedded cable
	// number. This is what the positional fallback below pairs against.
	outsByUnit := map[string][]portName{}
	for _, out := range outs {
		unit, _, _ := unitKeyOf(out.Name)
		outsByUnit[unit] = append(outsByUnit[unit], out)
	}
	for _, group := range outsByUnit {
		sort.Slice(group, func(i, j int) bool {
			_, _, ci := unitKeyOf(group[i].Name)
			_, _, cj := unitKeyOf(group[j].Name)
			return ci < cj
		})
	}

	type resolved struct {
		unit, role, outName string
		cable, outNum       int
	}
	consumedOut := map[string]bool{}
	byIn := make(map[string]*resolved, len(ins))
	var pending []portName

	// First pass: exact name matches. This is the reliable case —
	// CoreMIDI/ALSA, where a unit's in and out cables share a literal name,
	// and WinMM's cable 1 when it happens to. Claimed before the positional
	// fallback runs, so that fallback only ever fills in what this could not.
	for _, in := range ins {
		unit, role, cable := unitKeyOf(in.Name)
		r := &resolved{unit: unit, role: role, cable: cable, outNum: -1}
		byIn[in.Name] = r
		if out, ok := outByExactName[in.Name]; ok && !consumedOut[out.Name] {
			r.outName, r.outNum = out.Name, out.Num
			consumedOut[out.Name] = true
		} else {
			pending = append(pending, in)
		}
	}

	// Second pass: pair what is left by POSITION within the unit — the Nth
	// remaining input of a unit against the Nth remaining output of that
	// same unit, ordered by each side's own cable number — rather than by
	// matching the absolute embedded cable number between sides. WinMM
	// numbers MIDIOUT ports in a namespace entirely independent of MIDIIN's
	// (other MIDI-out devices on the system shift Push's own outputs to
	// different numbers than its inputs), so the previous "same absolute
	// cable number" key-based pairing left every cable but the exact-name
	// match unpaired on a real single-unit Windows machine — confirmed live
	// 2026-08-19: no Identify button ever appeared for the MIDI side, and a
	// forced connect failed with "no output cable paired with ...". This
	// fallback is scoped to one unit's own already-grouped outputs, so it
	// can never cross to a different physical unit's cable, and it is
	// skipped entirely for a unit with any ambiguous cable (see
	// ambiguousUnits) since pooling by name alone would mean pooling two
	// indistinguishable units' cables together.
	outPos := map[string]int{}
	for _, in := range pending {
		r := byIn[in.Name]
		if ambiguousUnits[r.unit] {
			continue
		}
		candidates := outsByUnit[r.unit]
		for outPos[r.unit] < len(candidates) && consumedOut[candidates[outPos[r.unit]].Name] {
			outPos[r.unit]++
		}
		if outPos[r.unit] < len(candidates) {
			out := candidates[outPos[r.unit]]
			r.outName, r.outNum = out.Name, out.Num
			consumedOut[out.Name] = true
			outPos[r.unit]++
		}
	}

	refs := make([]PortRef, 0, len(ins))
	for _, in := range ins {
		r := byIn[in.Name]
		k := cableKey{r.unit, r.cable}
		dev := pushmap.DeviceFromPortName(in.Name)
		isLive := r.role == "Live" || (r.role == "" && r.cable == 1)

		if ambiguousKeys[k] {
			refs = append(refs, PortRef{
				InName: in.Name, InNum: in.Num, OutNum: -1,
				Unit: r.unit, Cable: r.cable, Role: r.role, Device: dev,
				IsLive: isLive, Ambiguous: true,
			})
			continue
		}

		refs = append(refs, PortRef{
			InName: in.Name, InNum: in.Num, OutName: r.outName, OutNum: r.outNum,
			Unit: r.unit, Cable: r.cable, Role: r.role, Device: dev, IsLive: isLive,
		})
	}
	return refs
}

// ListPortRefs groups every Push MIDI cable the OS currently sees into
// PortRefs, pairing each input with its matching output.
func ListPortRefs() []PortRef {
	midilock.Lock()
	defer midilock.Unlock()

	var ins, outs []portName
	for _, p := range gm.GetInPorts() {
		if isPushHardwareName(p.String()) {
			ins = append(ins, portName{Name: p.String(), Num: p.Number()})
		}
	}
	for _, p := range gm.GetOutPorts() {
		if isPushHardwareName(p.String()) {
			outs = append(outs, portName{Name: p.String(), Num: p.Number()})
		}
	}
	return groupPorts(ins, outs)
}

// ListOutPortNames lists every Push-named MIDI output port with its driver
// number, unpaired. groupPorts never guesses a pairing for an ambiguous
// cable, so PortRef.OutNum is -1 for those — this raw list is what a caller
// falls back to for manual disambiguation, e.g. trying each candidate out
// port in turn with identify.FlashLEDs.
func ListOutPortNames() []string {
	midilock.Lock()
	defer midilock.Unlock()

	var names []string
	for _, p := range gm.GetOutPorts() {
		if isPushHardwareName(p.String()) {
			names = append(names, fmt.Sprintf("#%d %s", p.Number(), p.String()))
		}
	}
	return names
}

// ListUnits groups ListPortRefs by physical unit.
func ListUnits() []Unit {
	byKey := map[string]*Unit{}
	var order []string
	for _, ref := range ListPortRefs() {
		u, ok := byKey[ref.Unit]
		if !ok {
			u = &Unit{Key: ref.Unit, Device: ref.Device}
			byKey[ref.Unit] = u
			order = append(order, ref.Unit)
		}
		u.Ports = append(u.Ports, ref)
	}
	units := make([]Unit, len(order))
	for i, k := range order {
		units[i] = *byKey[k]
	}
	return units
}
