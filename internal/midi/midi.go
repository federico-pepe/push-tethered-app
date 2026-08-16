// Package midi handles Push's control surface over an OS MIDI API.
//
// It deliberately does NOT use libusb. Push's MIDI lives on interface 5, bound
// to the OS class driver — that binding is exactly why the CoreMIDI/ALSA ports
// exist. Claiming it over libusb would take Push's MIDI away from the DAW,
// which is full-ownership mode. In co-existence mode we must go through the OS.
// See docs/feasibility.md §6.1a.
//
// Decoding rule that matters: **branch on channel before CC**. Push 2 assigns
// CC 71-79 to the nine encoders, and CC 71/74 are also MPE timbre controllers.
// The numbers collide; the channel does not. Channel 1 is the control surface,
// channels 2-16 carry per-note MPE expression (§8.7).
package midi

import (
	"fmt"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
	gm "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // RtMidi C++ is vendored; no system package needed
)

// PortName is the only port that carries Push's control surface. User Port and
// External Port emit nothing but keepalive (§8.7).
const PortName = "Ableton Push 3 Live Port"

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
type Encoder struct {
	CC    byte
	Index int // 0-7 for encoders 1-8, -1 for the volume/tempo wheels
	Delta int
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
func Decode(b []byte) Event {
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
		if push3.IsEncoderCC(b[1]) {
			idx := -1
			if b[1] >= push3.CCEncoder1 && b[1] <= push3.CCEncoder8 {
				idx = int(b[1] - push3.CCEncoder1)
			}
			return Encoder{CC: b[1], Index: idx, Delta: push3.DecodeRel(b[2])}
		}
		name, _ := pushmap.ButtonName(b[1])
		return Button{CC: b[1], Name: name, Pressed: b[2] > 0}

	case 0x90, 0x80:
		if len(b) < 3 {
			return nil
		}
		on := status == 0x90 && b[2] > 0
		if push3.IsPadNote(b[1]) {
			col, row := push3.PadCoord(b[1])
			return Pad{Note: b[1], Col: col, Row: row, Channel: ch, Velocity: b[2], Pressed: on}
		}
		name, ok := pushmap.TouchName(b[1])
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
}

// Open connects to Push's Live Port for both input and LED output.
func Open() (*Port, error) {
	in, err := gm.FindInPort(PortName)
	if err != nil {
		return nil, fmt.Errorf("finding MIDI in %q: %w", PortName, err)
	}
	out, err := gm.FindOutPort(PortName)
	if err != nil {
		return nil, fmt.Errorf("finding MIDI out %q: %w", PortName, err)
	}
	send, err := gm.SendTo(out)
	if err != nil {
		return nil, fmt.Errorf("opening MIDI out: %w", err)
	}
	return &Port{in: in, out: out, send: send}, nil
}

// Listen starts delivering decoded events to fn until Close.
func (p *Port) Listen(fn func(Event)) error {
	stop, err := gm.ListenTo(p.in, func(msg gm.Message, _ int32) {
		if ev := Decode(msg); ev != nil {
			fn(ev)
		}
	})
	if err != nil {
		return fmt.Errorf("listening on %q: %w", PortName, err)
	}
	p.stop = stop
	return nil
}

// SetPad lights a grid pad. colour is a palette index from
// core/push3/colors.go; 0 turns the pad off.
func (p *Port) SetPad(note, colour byte) error {
	return p.send(gm.Message([]byte{0x90, note, colour}))
}

// SetButton sets a button LED's brightness (0-127). White button LEDs ignore
// colour, so the value is brightness rather than a palette index.
func (p *Port) SetButton(cc, brightness byte) error {
	return p.send(gm.Message([]byte{0xB0, cc, brightness}))
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
