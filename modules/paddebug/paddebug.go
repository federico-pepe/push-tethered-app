// Package paddebug is a throwaway diagnostic: it shows live per-pad
// slide/bend/pressure MPE data, to characterize whether that data stays
// continuous as a finger slides across adjacent pads (Push 3 only — Push 2
// has no MPE). Not meant to ship; delete once the padpointer module's
// design question is answered.
package paddebug

import (
	"fmt"

	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
)

// held tracks one currently-pressed pad's note and its live MPE readout.
type held struct {
	note              byte
	col, row          int
	channel           int
	velocity          byte
	slide, bend, pres int
}

// maxHeld bounds the on-screen list — Push 3 has 15 MPE member channels, but
// showing more than a handful at once would be unreadable.
const maxHeld = 8

type Module struct {
	host   module.Host
	device pushmap.Device
	held   []held // most-recently-pressed last
}

func New() *Module { return &Module{} }

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:          "paddebug",
		Name:        "Pad MPE Debug",
		Author:      "Federico Pepe",
		Version:     "0.1.0",
		Description: "Diagnostic: live slide/bend/pressure per held pad",
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	m.device = h.Device()
	h.Log("watching %v — slide a finger across adjacent pads and eyeball the readout", m.device)

	// Confirmed 2026-08-25 on real Push 3 hardware: pads stay on channel 1
	// (MPE off) here even after sending the standard MIDI MPE Configuration
	// Message (RPN 6, lower zone, master channel 1) on Init. Whatever turns
	// MPE on is not that — docs/protocol/live-handshake.md's undocumented
	// Ableton vendor SysEx (F0 00 21 1D 01 01 <cmd> ...), only observed while
	// Live is running, is the more likely gate. Not chased further here —
	// paddebug's job is done with channel pressure alone, which is real and
	// continuous regardless of MPE (see modules/padpointer).
	return nil
}

func (m *Module) Close() error { return nil }

func (m *Module) findByChannel(ch int) int {
	for i := range m.held {
		if m.held[i].channel == ch {
			return i
		}
	}
	return -1
}

func (m *Module) findByNote(note byte) int {
	for i := range m.held {
		if m.held[i].note == note {
			return i
		}
	}
	return -1
}

func (m *Module) Handle(ev module.Event) {
	switch e := ev.(type) {
	case module.Pad:
		if e.Pressed {
			m.host.Log("pad note=%d col=%d row=%d ch=%d vel=%d", e.Note, e.Col, e.Row, e.Channel, e.Velocity)
			h := held{note: e.Note, col: e.Col, row: e.Row, channel: e.Channel, velocity: e.Velocity}
			m.held = append(m.held, h)
			if len(m.held) > maxHeld {
				m.held = m.held[len(m.held)-maxHeld:]
			}
		} else if i := m.findByNote(e.Note); i >= 0 {
			m.held = append(m.held[:i], m.held[i+1:]...)
		}

	case module.Expression:
		m.host.Log("expression ch%d kind=%s value=%d", e.Channel, e.Kind, e.Value)
		if len(m.held) == 0 {
			return
		}
		// No MPE (Push 3 defaulting to channel 1, or Push 2): every pad
		// shares one channel, so there is no per-note channel to
		// disambiguate on — attribute the reading to whichever pad was
		// pressed most recently. With MPE, e.Channel would pick out the
		// exact note instead (findByChannel), but that path is unexercised
		// until MPE is confirmed active.
		i := len(m.held) - 1
		if j := m.findByChannel(e.Channel); j >= 0 && e.Channel != 1 {
			i = j
		}
		switch e.Kind {
		case "slide":
			m.held[i].slide = e.Value
		case "bend":
			m.held[i].bend = e.Value
		case "pressure":
			m.held[i].pres = e.Value
		}
	}
}

func (m *Module) Draw(f *module.Frame) {
	w, h := f.Size()
	t := m.host.Theme()

	f.Rect(0, 0, w, h, t.Black)
	f.Header(0, w, 20, "pushapp - paddebug")

	y := 30
	f.Text(10, y, "pad(col,row) ch  vel  slide  bend   pressure", t.Gray)
	y += 16
	if len(m.held) == 0 {
		f.Text(10, y, "press and hold pads, then slide across them", t.Gray)
	}
	for _, hd := range m.held {
		line := fmt.Sprintf("%3d (%d,%d)     ch%-2d vel%-3d  %-6d %-6d %-6d",
			hd.note, hd.col+1, hd.row+1, hd.channel, hd.velocity, hd.slide, hd.bend, hd.pres)
		f.Text(10, y, line, t.White)
		y += 16
	}

	status := "Push 3: expect continuous slide/bend across pad boundaries if truly hardware-sensed"
	if m.device == pushmap.Push2 {
		status = "Push 2 has no MPE — slide/bend/pressure will stay at 0, this is expected"
	}
	f.StatusBar(h-18, w, 18, status, false)
}
