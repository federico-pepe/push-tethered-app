// Package beatcount is the smallest possible demo of module.ExternalMIDI:
// it does nothing but count MIDI clock ticks and draw which beat of a 4/4
// bar it's on across the pad grid, as a digit.
//
// It exists as a working reference for NeedsMIDIIn — the counterpart to
// modules/thru's role for plain control-surface input — not as an
// instrument. modules/seq is the real consumer of external clock sync;
// this is the small version of the same idea, easy to read start to finish
// in one sitting.
package beatcount

import (
	"fmt"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

const (
	beats               = 4  // one bar, 4/4
	ticksPerQuarterNote = 24 // the MIDI clock standard, independent of tempo

	litPad = 120 // "white" pad, core/push3/colors.go
)

// digitBitmaps[i] is the glyph for beat i+1 (1-4). Each element is one row,
// top of the digit first; each bit is one column, bit 7 (leftmost in the
// literal) is column 0. Drawn onto the grid top-to-bottom, so row 0 here
// lands on the physical top row — see rowNote.
var digitBitmaps = [beats][8]byte{
	{ // 1
		0b_00011000,
		0b_00111000,
		0b_00011000,
		0b_00011000,
		0b_00011000,
		0b_00011000,
		0b_00011000,
		0b_01111110,
	},
	{ // 2
		0b_00111100,
		0b_01100110,
		0b_00000110,
		0b_00001100,
		0b_00011000,
		0b_00110000,
		0b_01100000,
		0b_01111110,
	},
	{ // 3
		0b_01111100,
		0b_00000110,
		0b_00000110,
		0b_00111100,
		0b_00000110,
		0b_00000110,
		0b_00000110,
		0b_01111100,
	},
	{ // 4
		0b_00001100,
		0b_00011100,
		0b_00110100,
		0b_01100100,
		0b_01111110,
		0b_00000100,
		0b_00000100,
		0b_00001100,
	},
}

// Module counts MIDI clock ticks and draws the current beat (1-4) as a
// digit across the pad grid.
type Module struct {
	host module.Host

	beat      int // 0-3
	tick      int // 0-(ticksPerQuarterNote-1) within the current beat
	haveClock bool
}

// New returns the beat counter module.
func New() *Module { return &Module{} }

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:          "beatcount",
		Name:        "Beat Counter (Clock Test)",
		Author:      "Federico Pepe",
		Version:     "1.0.0",
		Description: "counts an external MIDI clock, draws the beat (1-4) on the pad grid",
		NeedsMIDIIn: true,
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	h.Log("waiting for an external MIDI clock — point something's MIDI out at this app's input port")
	return nil
}

// Close blanks the grid. The host clears every LED on module switch/shutdown
// regardless, but doing it here too costs nothing and keeps the module
// honest about what it lit.
func (m *Module) Close() error {
	m.clearGrid()
	return nil
}

func (m *Module) Handle(ev module.Event) {
	e, ok := ev.(module.ExternalMIDI)
	if !ok || len(e.Raw) == 0 {
		return
	}
	switch e.Raw[0] {
	case 0xF8: // Timing Clock
		m.onClock()
	case 0xFA: // Start
		m.reset()
	}
}

func (m *Module) onClock() {
	if !m.haveClock {
		m.reset()
		return
	}
	m.tick++
	if m.tick < ticksPerQuarterNote {
		return
	}
	m.tick = 0
	m.beat = (m.beat + 1) % beats
	m.drawDigit(m.beat)
}

func (m *Module) reset() {
	m.tick = 0
	m.beat = 0
	m.haveClock = true
	m.drawDigit(m.beat)
}

// rowNote is the note for column col of physical row row (0 = bottom, see
// push3.PadNote) — the pad-grid mirror of a screen coordinate.
func rowNote(col, row int) byte { return push3.PadNote(col, row) }

// drawDigit lights every pad on the grid to match digitBitmaps[beat],
// clearing everything else — a full redraw rather than tracking a diff,
// which for 64 pads costs nothing and can never drift from what's actually
// meant to be showing.
func (m *Module) drawDigit(beat int) {
	bitmap := digitBitmaps[beat]
	for writtenRow := 0; writtenRow < 8; writtenRow++ {
		physicalRow := 7 - writtenRow // written row 0 is the top of the glyph
		rowBits := bitmap[writtenRow]
		for col := 0; col < 8; col++ {
			lit := rowBits&(1<<(7-col)) != 0
			colour := byte(0)
			if lit {
				colour = litPad
			}
			m.host.SetPad(rowNote(col, physicalRow), colour)
		}
	}
}

func (m *Module) clearGrid() {
	for col := 0; col < 8; col++ {
		for row := 0; row < 8; row++ {
			m.host.SetPad(rowNote(col, row), 0)
		}
	}
}

func (m *Module) Draw(f *module.Frame) {
	w, h := f.Size()
	t := m.host.Theme()
	f.Rect(0, 0, w, h, t.Black)

	f.Rect(0, 0, w, 20, t.CrumbBg)
	f.Text(8, 14, "pushapp - beat counter", t.CrumbCol)

	if !m.haveClock {
		f.Text(8, 60, "waiting for an external MIDI clock...", t.Gray)
		return
	}
	f.Text(8, 60, fmt.Sprintf("beat %d / %d", m.beat+1, beats), t.White)
}
