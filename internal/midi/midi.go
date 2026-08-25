// Package midi handles Push's control surface over an OS MIDI API.
//
// It deliberately does NOT use libusb. Push's MIDI lives on interface 5, bound
// to the OS class driver — that binding is exactly why the CoreMIDI/ALSA ports
// exist. Claiming it over libusb would take Push's MIDI away from the DAW,
// which is full-ownership mode. In co-existence mode we must go through the OS.
// See docs/archive/feasibility.md §6.1a.
//
// Decoding rule that matters: **branch on channel before CC**. Push 2 assigns
// CC 71-79 to the nine encoders, and CC 71/74 are also MPE timbre controllers.
// The numbers collide; the channel does not. Channel 1 is the control surface,
// channels 2-16 carry per-note MPE expression (§8.7).
package midi

import (
	"fmt"
	"strings"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/midilock"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
	gm "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // RtMidi C++ is vendored; no system package needed
)

// livePortSuffix identifies the port that carries the control surface. Push
// exposes several ports — Push 3 has Live/User/External, Push 2 has Live/User —
// but only the Live port carries pads, buttons and encoders (§8.7).
//
// Matched by substring rather than hardcoded, so the same code finds
// "Ableton Push 2 Live Port" and "Ableton Push 3 Live Port".
const livePortSuffix = "Live Port"

// hardwareNameMarker is what every real Push port name starts with, on every
// OS this repo has seen a capture from — "Ableton Push 2 Live Port",
// "Ableton Push 3 MIDI", "MIDIIN2 (Ableton Push 3 MIDI)". A bare "Push"
// substring is not enough: this app's own virtual MIDI-out port is named
// "Push Tethered App" (internal/midiout.DefaultName), and every "contains
// Push" filter in this package would otherwise mistake it for a hardware
// unit's own cable — confirmed live 2026-08-19, where pairing a real unit
// while a session with a MIDI-out module active caused "Push Tethered App"
// to show up in the pairing view as if it were a Push.
const hardwareNameMarker = "Ableton Push"

// isPushHardwareName reports whether name looks like a real Push port, not
// merely a port that happens to mention "Push" — see hardwareNameMarker.
func isPushHardwareName(name string) bool {
	return strings.Contains(name, hardwareNameMarker)
}

// findPort returns the first port whose name mentions Push and the Live port.
func findPortName(names []string) (string, error) {
	for _, n := range names {
		if isPushHardwareName(n) && strings.Contains(n, livePortSuffix) {
			return n, nil
		}
	}
	return "", fmt.Errorf("no Push Live Port found among %v "+
		"(connected, and in controller mode?)", names)
}

// Event is any decoded control-surface or pad event.
type Event interface{ eventName() string }

// Button is a press or release of a CC button on channel 1.
type Button struct {
	CC      byte
	Name    string // "" when the CC is not in the known map
	Pressed bool
}

func (Button) eventName() string { return "button" }

// Encoder is a relative encoder turn on channel 1. Delta is already decoded
// via push3.DecodeRel and can exceed ±1: the encoders accelerate, with deltas
// up to ±11 observed on fast turns (§8.8).
//
// "Encoder" includes the volume, tempo and jog wheels, which use the same
// relative encoding. Index identifies the eight numbered encoders; the wheels
// are distinguished by CC (see Name).
type Encoder struct {
	CC     byte
	Index  int // 0-7 for encoders 1-8, -1 for the volume/tempo/swing/jog wheels
	Delta  int
	Device pushmap.Device
}

// Name returns a human label for the encoder or wheel. Device-aware, since
// Push 2's CC 15 is the Swing encoder and Push 3 has none (§11).
func (e Encoder) Name() string {
	if e.Index >= 0 {
		return fmt.Sprintf("encoder %d", e.Index+1)
	}
	if n, ok := pushmap.ButtonNameFor(e.Device, e.CC); ok {
		return n
	}
	return fmt.Sprintf("CC %d", e.CC)
}

func (Encoder) eventName() string { return "encoder" }

// Touch is a capacitive touch sensor on channel 1. Note numbers come from
// internal/pushmap, not core/push3, whose values are off by one (§8.8).
type Touch struct {
	Note    byte
	Name    string
	Touched bool
}

func (Touch) eventName() string { return "touch" }

// Pad is a grid pad press or release. With MPE on (the default), Channel is
// the note's member channel, 2-16.
type Pad struct {
	Note     byte
	Col, Row int // 0-indexed from bottom-left
	Channel  int
	Velocity byte
	Pressed  bool
}

func (Pad) eventName() string { return "pad" }

// Expression is per-note MPE data: pressure, slide (CC 74) or pitch bend, on
// the note's member channel.
type Expression struct {
	Channel int
	Kind    string // "pressure", "slide", "bend"
	Value   int
}

func (Expression) eventName() string { return "expression" }

// Decode turns raw MIDI bytes into an Event, or nil if the message is not
// interesting (system realtime, unknown status).
//
// System realtime is tested BEFORE masking with 0xF0: Push emits Active
// Sensing (0xFE) about 37 times a second, and 0xFE & 0xF0 == 0xF0, so masking
// first makes keepalive look like SysEx (§8.7).
func Decode(b []byte) Event { return DecodeFor(pushmap.Push3, b) }

// DecodeFor decodes for a specific device. Push 2 and Push 3 share most of the
// map but differ on a handful of controls (§11), so the device matters for
// naming and for which CCs count as encoders.
func DecodeFor(d pushmap.Device, b []byte) Event {
	if len(b) < 2 || b[0] >= 0xF8 {
		return nil
	}
	status, ch := b[0]&0xF0, int(b[0]&0x0F)+1

	if ch != 1 {
		return decodeMPE(status, ch, b)
	}

	switch status {
	case 0xB0:
		if len(b) < 3 {
			return nil
		}
		if pushmap.IsRelativeEncoderCCFor(d, b[1]) {
			idx := -1
			if b[1] >= push3.CCEncoder1 && b[1] <= push3.CCEncoder8 {
				idx = int(b[1] - push3.CCEncoder1)
			}
			return Encoder{CC: b[1], Index: idx, Delta: push3.DecodeRel(b[2]), Device: d}
		}
		name, _ := pushmap.ButtonNameFor(d, b[1])
		return Button{CC: b[1], Name: name, Pressed: b[2] > 0}

	case 0xD0:
		// Channel (not per-note) pressure — present even without MPE
		// (confirmed 2026-08-25 on real Push 3 hardware: continuous, ramps
		// smoothly while a pad is held, distinct from the pad's Velocity at
		// note-on). With MPE off there is no per-note channel to attribute
		// this to, so it applies to whichever pad(s) are currently held —
		// a module has to decide how to attribute it itself.
		if len(b) < 2 {
			return nil
		}
		return Expression{Channel: ch, Kind: "pressure", Value: int(b[1])}

	case 0x90, 0x80:
		if len(b) < 3 {
			return nil
		}
		on := status == 0x90 && b[2] > 0
		if push3.IsPadNote(b[1]) {
			col, row := push3.PadCoord(b[1])
			return Pad{Note: b[1], Col: col, Row: row, Channel: ch, Velocity: b[2], Pressed: on}
		}
		name, ok := pushmap.TouchNameFor(d, b[1])
		if !ok {
			name = fmt.Sprintf("unknown note %d", b[1])
		}
		return Touch{Note: b[1], Name: name, Touched: on}
	}
	return nil
}

func decodeMPE(status byte, ch int, b []byte) Event {
	switch status {
	case 0x90, 0x80:
		if len(b) < 3 {
			return nil
		}
		on := status == 0x90 && b[2] > 0
		if !push3.IsPadNote(b[1]) {
			return nil
		}
		col, row := push3.PadCoord(b[1])
		return Pad{Note: b[1], Col: col, Row: row, Channel: ch, Velocity: b[2], Pressed: on}
	case 0xD0:
		return Expression{Channel: ch, Kind: "pressure", Value: int(b[1])}
	case 0xB0:
		if len(b) < 3 {
			return nil
		}
		return Expression{Channel: ch, Kind: "slide", Value: int(b[2])}
	case 0xE0:
		if len(b) < 3 {
			return nil
		}
		return Expression{Channel: ch, Kind: "bend", Value: int(b[1]) | int(b[2])<<7}
	}
	return nil
}

// Port is an open bidirectional connection to Push's Live Port.
type Port struct {
	in   drivers.In
	out  drivers.Out
	send func(gm.Message) error
	stop func()
	name string
	ref  PortRef
	dev  pushmap.Device
}

// Device reports which Push this port belongs to, inferred from its name.
func (p *Port) Device() pushmap.Device { return p.dev }

// Name returns the MIDI port this connection uses.
func (p *Port) Name() string { return p.name }

// Ref returns the PortRef this connection was opened from. Zero-valued for a
// Port opened before PortRef existed in this process's history — none is,
// since every open path now goes through OpenRef.
func (p *Port) Ref() PortRef { return p.ref }

// ListInPorts returns the name of every MIDI input port the OS currently
// sees, Push or not. Exists so a caller that can't auto-detect the Live port
// (§ Windows naming below) can offer the user a manual pick.
func ListInPorts() []string {
	midilock.Lock()
	defer midilock.Unlock()

	var names []string
	for _, p := range gm.GetInPorts() {
		names = append(names, p.String())
	}
	return names
}

// Open connects to Push's Live Port for both input and LED output, guessing
// which port that is by name.
//
// The guess is unreliable on Windows: CoreMIDI and ALSA read the "Live
// Port"/"User Port"/"External Port" strings straight from the device's own
// USB MIDI jack descriptors, but WinMM does not expose jack strings at all —
// it names the first cable after the bare device name and prefixes only the
// others ("Ableton Push 3 MIDI", "MIDIIN2 (Ableton Push 3 MIDI)", "MIDIIN3
// (...)"), so livePortSuffix never matches there. Found 2026-08-18 from a
// real Windows report. OpenNamed exists as the escape hatch — a caller with
// no better guess should list ListInPorts() and let the user pick.
//
// Refuses when more than one Push is attached: with two units there is no
// name-based way to tell which one's Live Port this would open, and guessing
// risks silently driving the wrong physical unit. Callers that need to pick
// among several units should use ListUnits and OpenRef instead.
func Open() (*Port, error) {
	units := ListUnits()
	if len(units) > 1 {
		return nil, fmt.Errorf("%d Push units attached — pass a PortRef from ListUnits instead of auto-detecting", len(units))
	}
	name, err := findPortName(ListInPorts())
	if err != nil {
		return nil, err
	}
	return OpenNamed(name)
}

// OpenNamed connects to the given MIDI port by name for input, and to the
// matching output cable for LED output, skipping the Live-port guess Open
// makes. Use when the caller already knows which port is Push's Live port —
// typically because the user picked it, after Open's heuristic failed.
//
// Refuses when name matches more than one unit's cables rather than silently
// opening whichever the driver lists first — see ListPortRefs.
func OpenNamed(name string) (*Port, error) {
	var matches []PortRef
	for _, ref := range ListPortRefs() {
		if ref.InName == name {
			matches = append(matches, ref)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no Push input port named %q", name)
	case 1:
		return OpenRef(matches[0])
	default:
		return nil, fmt.Errorf("%q matches %d units — ambiguous, use OpenRef with a specific PortRef", name, len(matches))
	}
}

// OpenRef opens exactly the cable ref names, by driver port number rather than
// by name. Opening by number is what makes the second of two identically
// named units reachable at all: gomidi's name-based lookup does a substring
// match and returns whichever port it finds first (drivers/port.go), so two
// Push units sharing a name are otherwise indistinguishable through it.
//
// Re-validates that the port at ref.InNum still carries ref.InName before
// opening — if it does not, the port list changed since ref was produced (a
// unit was unplugged, most likely), and opening by number alone would risk
// opening whatever now sits at that slot instead of the unit the caller meant.
func OpenRef(ref PortRef) (*Port, error) {
	if ref.Ambiguous {
		return nil, fmt.Errorf("port %q is ambiguous — two units produced indistinguishable names; identify the out cable by its LEDs first", ref.InName)
	}
	if ref.OutNum < 0 {
		return nil, fmt.Errorf("no output cable paired with %q", ref.InName)
	}

	midilock.Lock()
	defer midilock.Unlock()

	current := map[int]string{}
	for _, p := range gm.GetInPorts() {
		current[p.Number()] = p.String()
	}
	if got := current[ref.InNum]; got != ref.InName {
		return nil, fmt.Errorf("MIDI port list changed: port %d is now %q, expected %q — re-enumerate and retry",
			ref.InNum, got, ref.InName)
	}

	in, err := gm.InPort(ref.InNum)
	if err != nil {
		return nil, fmt.Errorf("opening MIDI in %q (port %d): %w", ref.InName, ref.InNum, err)
	}
	out, err := gm.OutPort(ref.OutNum)
	if err != nil {
		return nil, fmt.Errorf("opening MIDI out %q (port %d): %w", ref.OutName, ref.OutNum, err)
	}
	send, err := gm.SendTo(out)
	if err != nil {
		return nil, fmt.Errorf("opening MIDI out: %w", err)
	}
	return &Port{in: in, out: out, send: send, name: ref.InName, ref: ref, dev: ref.Device}, nil
}

// Listen starts delivering decoded events to fn until Close.
func (p *Port) Listen(fn func(Event)) error {
	stop, err := gm.ListenTo(p.in, func(msg gm.Message, _ int32) {
		if ev := DecodeFor(p.dev, msg); ev != nil {
			fn(ev)
		}
	})
	if err != nil {
		return fmt.Errorf("listening on %q: %w", p.name, err)
	}
	p.stop = stop
	return nil
}

// SetPad lights a grid pad. colour is a palette index from
// core/push3/colors.go; 0 turns the pad off.
func (p *Port) SetPad(note, colour byte) error {
	return p.send(gm.Message([]byte{0x90, note, colour}))
}

// SetButton lights a button LED. value is a palette index, same mechanism
// and same palette as SetPad's colour — confirmed 2026-08-18 on the
// screen-adjacent buttons (see docs/protocol/led-output.md); not a brightness
// scale, despite this parameter's name in callers that predate that finding.
func (p *Port) SetButton(cc, value byte) error {
	return p.send(gm.Message([]byte{0xB0, cc, value}))
}

// Clear turns off every pad and every mapped button LED. Always call this on
// shutdown — a device left lit makes the next run ambiguous.
func (p *Port) Clear() {
	for n := byte(push3.PadNoteMin); n <= push3.PadNoteMax; n++ {
		_ = p.SetPad(n, 0)
	}
	for _, cc := range pushmap.LitButtons() {
		_ = p.SetButton(cc, 0)
	}
}

// OutCable is a bare LED-writing connection to a single MIDI out cable, with
// no paired input. It exists for internal/identify's FlashLEDs: identifying a
// candidate out cable is exactly the operation needed to resolve a PortRef
// the pairing logic marked Ambiguous, so it must not require — or refuse
// because of — the same pairing OpenRef enforces for a full bidirectional
// Port.
type OutCable struct {
	out  drivers.Out
	send func(gm.Message) error
}

// OpenOutCable opens outNum for LED writes only. Unlike OpenRef this performs
// no re-validation against a remembered name and no ambiguity check — the
// caller is expected to already be choosing among candidates by number,
// which is exactly the situation where a name-based or paired lookup has
// already given up.
func OpenOutCable(outNum int) (*OutCable, error) {
	midilock.Lock()
	defer midilock.Unlock()

	out, err := gm.OutPort(outNum)
	if err != nil {
		return nil, fmt.Errorf("opening MIDI out port %d: %w", outNum, err)
	}
	send, err := gm.SendTo(out)
	if err != nil {
		return nil, fmt.Errorf("opening MIDI out port %d: %w", outNum, err)
	}
	return &OutCable{out: out, send: send}, nil
}

// SetPad lights a grid pad. Same encoding as Port.SetPad.
func (c *OutCable) SetPad(note, colour byte) error {
	return c.send(gm.Message([]byte{0x90, note, colour}))
}

// Clear turns off every pad.
func (c *OutCable) Clear() {
	for n := byte(push3.PadNoteMin); n <= push3.PadNoteMax; n++ {
		_ = c.SetPad(n, 0)
	}
}

// Close clears every pad this cable lit and releases it.
func (c *OutCable) Close() {
	c.Clear()
	if c.out != nil && c.out.IsOpen() {
		_ = c.out.Close()
	}
}

// Close stops listening, clears the LEDs and releases the ports.
func (p *Port) Close() {
	if p.stop != nil {
		p.stop()
	}
	p.Clear()
	if p.out != nil && p.out.IsOpen() {
		_ = p.out.Close()
	}
	if p.in != nil && p.in.IsOpen() {
		_ = p.in.Close()
	}
}
