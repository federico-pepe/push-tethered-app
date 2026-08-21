// Package moduletest provides a fake module.Host so modules can be tested with
// no Push attached.
//
// Modules are the part of this project most people will write, so they have to
// be testable without hardware — a module author should be able to assert "this
// pad press lights that LED and sends that note" in a unit test. The fake
// records everything a module does to the host, in order.
package moduletest

import (
	"encoding/json"
	"fmt"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
	"github.com/federico-pepe/push-tethered-app/internal/renderframe"
)

// PadWrite is one recorded SetPad call.
type PadWrite struct {
	Note   byte
	Colour byte
}

// ButtonWrite is one recorded SetButton call.
type ButtonWrite struct {
	CC         byte
	Brightness byte
}

// MIDIWrite is one recorded outbound MIDI message.
type MIDIWrite struct {
	Kind         string // "cc" | "note" | "noteoff"
	Ch, Num, Val byte
}

// Host is a fake module.Host. The zero value works: it behaves as a Push 3 with
// the default theme and a MIDI output port available.
type Host struct {
	// Dev is the device modules see. Zero value is pushmap.Push3.
	Dev pushmap.Device

	// NoMIDIOut makes the Send* methods fail, so a module's error handling can
	// be tested without arranging a real port.
	NoMIDIOut bool

	// Ops overrides the reported SupportedOps list. Nil means "everything the
	// built-in renderer knows", spelled out here rather than imported from
	// internal/host to keep this package free of the hardware side.
	Ops []string

	Pads    []PadWrite
	Buttons []ButtonWrite
	MIDI    []MIDIWrite
	Logs    []string

	store []byte
}

var _ module.Host = (*Host)(nil)

func (h *Host) Device() pushmap.Device { return h.Dev }
func (h *Host) Theme() module.Theme    { return widgets.Default }

// SupportedOps defers to internal/renderframe's real registry rather than a
// hand-duplicated list — renderframe was split out of internal/host
// precisely so a gousb-free caller like this one could import it directly,
// which makes keeping a second copy in sync pure risk with no upside.
func (h *Host) SupportedOps() []string {
	if h.Ops != nil {
		return h.Ops
	}
	return renderframe.SupportedOps()
}

func (h *Host) SetPad(note, colour byte) {
	h.Pads = append(h.Pads, PadWrite{Note: note, Colour: colour})
}

func (h *Host) SetButton(cc, brightness byte) {
	h.Buttons = append(h.Buttons, ButtonWrite{CC: cc, Brightness: brightness})
}

func (h *Host) SendCC(ch, cc, val byte) error {
	return h.record("cc", ch, cc, val)
}

func (h *Host) SendNote(ch, note, vel byte) error {
	return h.record("note", ch, note, vel)
}

func (h *Host) NoteOff(ch, note byte) error {
	return h.record("noteoff", ch, note, 0)
}

func (h *Host) SendClock() error    { return h.record("clock", 0, 0, 0) }
func (h *Host) SendStart() error    { return h.record("start", 0, 0, 0) }
func (h *Host) SendContinue() error { return h.record("continue", 0, 0, 0) }
func (h *Host) SendStop() error     { return h.record("stop", 0, 0, 0) }

func (h *Host) record(kind string, ch, num, val byte) error {
	if h.NoMIDIOut {
		return fmt.Errorf("no MIDI output port is open")
	}
	h.MIDI = append(h.MIDI, MIDIWrite{Kind: kind, Ch: ch, Num: num, Val: val})
	return nil
}

func (h *Host) Log(format string, args ...any) {
	h.Logs = append(h.Logs, fmt.Sprintf(format, args...))
}

func (h *Host) Store() module.Store { return (*store)(h) }

// store implements module.Store over the fake's byte slice, so persistence
// round-trips within a test without touching disk.
type store Host

func (s *store) Get(v any) error {
	if len(s.store) == 0 {
		return nil // nothing stored yet; leave v (and its defaults) alone
	}
	return json.Unmarshal(s.store, v)
}

func (s *store) Set(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.store = b
	return nil
}

// LitPads returns the pads currently lit, folding the recorded writes in order
// so a colour-then-off pair cancels out. Saves every test reimplementing it.
func (h *Host) LitPads() map[byte]byte {
	lit := map[byte]byte{}
	for _, w := range h.Pads {
		if w.Colour == 0 {
			delete(lit, w.Note)
			continue
		}
		lit[w.Note] = w.Colour
	}
	return lit
}

// Reset clears the recorded calls, keeping the configuration.
func (h *Host) Reset() {
	h.Pads = nil
	h.Buttons = nil
	h.MIDI = nil
	h.Logs = nil
}
