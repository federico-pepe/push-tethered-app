package midi

import (
	"fmt"
	"regexp"
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

// unitKeyOf splits a port name into the unit it belongs to, the cable's named
// role if it has one, and its 1-based position among that unit's cables.
//
// Three shapes are handled, in order: CoreMIDI/ALSA role-suffixed names (which
// carry the true role), WinMM's wrapped names for cable 2 and up, and WinMM's
// bare device name for cable 1. Anything else is treated as a single
// unnamed-role cable 1, which keeps today's single-unit behaviour rather than
// failing outright on a naming convention this code has not seen yet.
func unitKeyOf(name string) (unit, role string, cable int) {
	for _, suffix := range roleSuffixes {
		if strings.HasSuffix(name, suffix) {
			unit = strings.TrimSpace(strings.TrimSuffix(name, suffix))
			role = strings.TrimSuffix(suffix, " Port")
			return unit, role, roleOrder[role]
		}
	}

	if m := winmmWrapped.FindStringSubmatch(name); m != nil {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		return m[2], "", n
	}

	return name, "", 1
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

	outByExactName := map[string]portName{}
	outByKey := map[cableKey]portName{}
	for _, out := range outs {
		outByExactName[out.Name] = out
		unit, _, cable := unitKeyOf(out.Name)
		outByKey[cableKey{unit, cable}] = out
	}

	refs := make([]PortRef, 0, len(ins))
	for _, in := range ins {
		unit, role, cable := unitKeyOf(in.Name)
		k := cableKey{unit, cable}
		dev := pushmap.DeviceFromPortName(in.Name)
		isLive := role == "Live" || (role == "" && cable == 1)

		if ambiguousKeys[k] {
			refs = append(refs, PortRef{
				InName: in.Name, InNum: in.Num, OutNum: -1,
				Unit: unit, Cable: cable, Role: role, Device: dev,
				IsLive: isLive, Ambiguous: true,
			})
			continue
		}

		ref := PortRef{
			InName: in.Name, InNum: in.Num, OutNum: -1,
			Unit: unit, Cable: cable, Role: role, Device: dev, IsLive: isLive,
		}
		// Prefer an exact name match — the common case on CoreMIDI/ALSA,
		// where in and out cables of one unit share a literal name — over the
		// key-based pairing WinMM needs.
		if out, ok := outByExactName[in.Name]; ok {
			ref.OutName, ref.OutNum = out.Name, out.Num
		} else if out, ok := outByKey[k]; ok {
			ref.OutName, ref.OutNum = out.Name, out.Num
		}
		refs = append(refs, ref)
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
