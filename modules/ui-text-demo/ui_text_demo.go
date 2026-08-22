// Package uitextdemo is a live font-tuning bench: every encoder drives one
// text parameter (face, weight, size, color, margin) so the Tamzen/Helvetica
// Neue swap can be dialed in on real hardware instead of guessing constants
// and rebuilding cmd/screensim scenes each time.
//
// Controls:
//   - Encoder 1: face — Basic (Tamzen, fixed size/weight) vs Styled
//     (Helvetica Neue, honors weight/size below)
//   - Encoder 2: weight — Regular/Bold/Italic/BoldItalic (Styled face only)
//   - Encoder 3: size in points (Styled face only)
//   - Encoder 4: color — cycles the 0-127 Push hardware LED palette
//     (core/push3.Palette/ColorForIndex) instead of dialing raw RGB
//   - Encoder 5: left margin (x)
//   - Encoder 6: vertical offset from screen center (y)
//   - Encoders 7-8: unused, reserved for future parameters
//   - Bottom soft-buttons 1-4: pick a sample string
//   - Bottom soft-button 8: reset every parameter to its default
package uitextdemo

import (
	"fmt"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

var samples = [4]string{
	"The Quick Brown Fox 0123",
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ",
	"Push Tethered App",
	"!@#$%^&*()_+-=[]",
}

var weightNames = [4]string{"Regular", "Bold", "Italic", "BoldItalic"}
var weights = [4]module.Weight{module.Regular, module.Bold, module.Italic, module.BoldItalic}

// Module holds all state as plain fields — Handle and Draw never run
// concurrently, so no locking, the same contract every module in this repo
// follows.
type Module struct {
	host module.Host

	enc [8]int // raw accumulated encoder deltas, one slot per encoder

	sample int // index into samples, chosen by bottom soft-buttons 0-3
}

// New returns the UI text-tuning demo module.
func New() *Module {
	return &Module{}
}

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:          "ui-text-demo",
		Name:        "UI Text Demo",
		Author:      "push-tethered-app",
		Description: "live-tune face/weight/size/color/margin on real hardware",
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	return nil
}

func (m *Module) Close() error {
	return nil
}

// ── input ────────────────────────────────────────────────────────────────

func (m *Module) Handle(ev module.Event) {
	switch e := ev.(type) {
	case module.Button:
		if !e.Pressed {
			return
		}
		for i := range 8 {
			if e.CC != push3.CCScreenBotN(i) {
				continue
			}
			switch {
			case i >= 0 && i <= 3:
				m.sample = i
			case i == 7:
				m.enc = [8]int{}
				m.sample = 0
			}
		}

	case module.Encoder:
		if e.Index >= 0 && e.Index < len(m.enc) {
			m.enc[e.Index] += e.Delta
			// Encoders 3, 5, 6 drive bounded values (size, x/y margin): clamp
			// at write time so an endless encoder stops at the limit instead
			// of wrapping, and a reversal past the limit responds
			// immediately instead of having to unwind however far past the
			// edge it had accumulated. Encoders 1, 2, 4 (face/weight/color)
			// are discrete pickers meant to cycle, so they stay unclamped
			// and wrap in Draw below.
			switch e.Index {
			case 2: // size: 8-48pt, raw+12 in [0,40]
				m.enc[2] = push3.ClampInt(m.enc[2], -12, 28)
			case 4: // margin X: default 10, raw+10 in [0,199]
				m.enc[4] = push3.ClampInt(m.enc[4], -10, 189)
			case 5: // margin Y: -60..+60, raw+60 in [0,120]
				m.enc[5] = push3.ClampInt(m.enc[5], -60, 60)
			}
		}
	}
}

// wrap folds raw into [0, mod), taking the sign of Go's % into account —
// encoder deltas are unclamped and can go negative. For a discrete picker
// (face, weight, color) cycling back around is the expected behavior of
// "turn to the next option"; for a bounded value (size, margin) use
// push3.ClampInt instead so the endless encoder stops at the limit rather
// than wrapping past it.
func wrap(raw, mod int) int {
	v := raw % mod
	if v < 0 {
		v += mod
	}
	return v
}

// ── drawing ──────────────────────────────────────────────────────────────

func (m *Module) Draw(f *module.Frame) {
	w, h := f.Size()
	t := m.host.Theme()
	f.Rect(0, 0, w, h, t.Black)

	// Every formula is centered so all-zero encoders (the module's resting
	// state on activation) land on a sane default: white, mid-size, centered
	// text — not black-on-black pinned off the top edge.
	basic := wrap(m.enc[0]/8, 2) == 0
	weightIdx := wrap(m.enc[1]/4, 4)
	size := float64(8 + push3.ClampInt(m.enc[2]+12, 0, 40)) // 8-48pt, default 20
	// +120 so the resting position (all-zero encoders) lands on palette
	// index 120 ("white") instead of 0 ("off") — same reasoning as size and
	// margin below, just a starting offset into the 0-127 cycle, not a
	// different range.
	colorIdx := wrap(m.enc[3]+120, 128)
	pe := push3.ColorForIndex(byte(colorIdx))
	marginX := push3.ClampInt(m.enc[4]+10, 0, 199) // default 10
	marginY := push3.ClampInt(m.enc[5]+60, 0, 120) - 60

	col := pe.RGB
	s := samples[m.sample]

	faceName := "Basic (Tamzen)"
	if !basic {
		faceName = "Styled (Helvetica Neue)"
	}
	f.Header(0, w, 20, "pushapp - ui-text-demo")
	f.Text(4, 36, fmt.Sprintf(
		"FACE=%s  WEIGHT=%s  SIZE=%.0fpt  COLOR=%d %s  MX=%d  MY=%+d",
		faceName, weightNames[weightIdx], size, colorIdx, pe.Name, marginX, marginY,
	), t.Gray)

	baseline := h/2 + marginY
	if basic {
		f.Text(marginX, baseline, s, col)
	} else {
		f.StyledText(marginX, baseline, s, col, weights[weightIdx], size)
	}

	f.StatusBar(h-18, w, 18, "BTN 1-4: sample text   BTN 8: reset", false)
}
