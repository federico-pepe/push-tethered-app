// Package uidemo exercises every widget in the design system, each driven
// by a real hardware control, on one page per widget cluster.
//
// It exists to verify the design system on real Push hardware — every
// other proof so far (core/gfx/widgets' own unit tests, cmd/screensim's
// scenes) checks that a widget renders correctly given hand-built input.
// None of that proves an encoder turn, a pad press or a D-Pad press
// actually reaches the widget and changes what's on screen the way a
// person expects — this module is what a person runs on real hardware,
// Live closed, to eyeball that end-to-end path per control:
//
//   - D-Pad left/right: change page
//   - Encoders 1-8: live values feeding whichever widgets the current page
//     shows (a knob, a meter, a fader, an envelope point, list cursors)
//   - The 8 pads: toggle cells on the pad-grid page, mirrored to their own
//     LEDs via Host.SetPad — proves the screen and the physical LED agree
//   - The 8 under-screen soft-buttons: toggle an exclusive (radio) group
//     and an independent-toggle group on the buttons page
//
// Not exercised: the jog wheel, the touch strip, MPE, and the top-row
// buttons — none of those have a widget in the catalog yet to verify.
package uidemo

import (
	"fmt"
	"image/color"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// NumPages is exported so tooling (cmd/screensim) can cycle every page from
// outside the package via ordinary D-Pad Handle calls, without reaching
// into unexported state.
const NumPages = 9
const numPages = NumPages

var pageNames = [numPages]string{
	"bars", "knobs", "meters", "lists", "buttons", "pad grid", "fonts", "helvetica", "envelope",
}

// Module holds all state as plain fields — Handle and Draw never run
// concurrently, so no locking, the same contract every module in this
// repo follows.
type Module struct {
	host module.Host

	page int

	// enc[i] is encoder i's raw accumulated delta — signed, unclamped;
	// each page interprets whichever encoders it uses however it needs to
	// (a knob wants a clamped 0-100 range, an envelope point wants 0-1).
	enc [8]int

	pads [8][8]bool // pad-grid page's toggle state, [row][col]

	quantize *module.ButtonGroup // exclusive: buttons page, cols 0-2
	toggles  *module.ButtonGroup // independent: buttons page, cols 4-5
}

// New returns the UI demo module.
func New() *Module {
	return &Module{
		quantize: module.NewButtonGroup(true),
		toggles:  module.NewButtonGroup(false),
	}
}

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:          "ui-demo",
		Name:        "UI Demo",
		Author:      "push-tethered-app",
		Description: "exercises every design-system widget from real hardware controls",
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
		switch e.CC {
		case push3.CCDPadLeft:
			m.page = (m.page - 1 + numPages) % numPages
		case push3.CCDPadRight:
			m.page = (m.page + 1) % numPages
		}
		for i := range 8 {
			if e.CC == push3.CCScreenBotN(i) {
				m.handleSoftButton(i)
			}
		}

	case module.Pad:
		if e.Pressed && e.Col < 8 && e.Row < 8 {
			m.pads[e.Row][e.Col] = !m.pads[e.Row][e.Col]
			colour := byte(0)
			if m.pads[e.Row][e.Col] {
				colour = 120 // white, matching monitor/thru's own padColour
			}
			m.host.SetPad(e.Note, colour)
		}

	case module.Encoder:
		if e.Index >= 0 && e.Index < len(m.enc) {
			m.enc[e.Index] += e.Delta
		}
	}
}

func (m *Module) handleSoftButton(i int) {
	switch {
	case i >= 0 && i <= 2:
		m.quantize.Toggle(i)
	case i == 4 || i == 5:
		m.toggles.Toggle(i)
	}
}

// clampedFrac maps an encoder's raw accumulated delta onto [0,1], wrapping
// every 100 clicks — a demo range, not a claim about real encoder
// sensitivity, which every module tunes for its own control.
func clampedFrac(raw int) float64 {
	v := raw % 100
	if v < 0 {
		v += 100
	}
	return float64(v) / 100
}

// ── drawing ──────────────────────────────────────────────────────────────

func (m *Module) Draw(f *module.Frame) {
	w, h := f.Size()
	t := m.host.Theme()
	f.Rect(0, 0, w, h, t.Black)

	title := fmt.Sprintf("pushapp - ui-demo  [%d/%d] %s  (D-Pad left/right to change page)",
		m.page+1, numPages, pageNames[m.page])
	f.Header(0, w, 20, title)

	switch pageNames[m.page] {
	case "bars":
		m.drawBars(f, w, h, t)
	case "knobs":
		m.drawKnobs(f, w, h, t)
	case "meters":
		m.drawMeters(f, w, h, t)
	case "lists":
		m.drawLists(f, w, h, t)
	case "buttons":
		m.drawButtons(f, w, h, t)
	case "pad grid":
		m.drawPadGrid(f, w, h, t)
	case "fonts":
		m.drawFonts(f, w, h, t)
	case "helvetica":
		m.drawHelvetica(f, w, h, t)
	case "envelope":
		m.drawEnvelope(f, w, h, t)
	}

	if pageNames[m.page] != "buttons" {
		f.StatusBar(h-18, w, 18, "turn encoders 1-8 to change values on this page", false)
	}
}

func (m *Module) drawBars(f *module.Frame, w, h int, t module.Theme) {
	f.Breadcrumb(24, w, "Header (above) + Breadcrumb (this bar)", "")
	f.StatusBar(h-18, w, 18, "StatusBar, normal state", false)
}

// Both knobs sit at cy=75, r=35: DrawKnobFull's own label lands at
// cy+r+24=134, DrawKnob's at cy+r+12=122 — both clear of the status bar's
// top edge at h-18=142. Learned by rendering this page and finding the
// labels drawn right on top of the bar with the first, more generous
// radius.
func (m *Module) drawKnobs(f *module.Frame, w, h int, t module.Theme) {
	v0 := clampedFrac(m.enc[0]) * 100
	v1 := clampedFrac(m.enc[1]) * 100
	f.Knob(140, 75, 35, module.Knob{Label: "Knob (enc 1)", Value: v0, Min: 0, Max: 100})
	f.KnobFull(360, 75, 35, module.Knob{Label: "KnobFull (enc 2)", Value: v1, Min: 0, Max: 100})
}

func (m *Module) drawMeters(f *module.Frame, w, h int, t module.Theme) {
	f.Meter(40, 40, 260, 14, clampedFrac(m.enc[2]), t.OnColor, t.DarkGray)
	f.Text(40, 70, "Meter (enc 3)", t.Gray)

	// h=90 not 100: MeterV draws no label of its own (the "MeterV (enc 4)"
	// text below is this scene's own), so 90 just leaves room for that text
	// above the status bar; DrawFader's *internal* label lands at y+h+12,
	// which is the real constraint — 30+90+12=132, clear of h-18=142.
	//
	// x=340 and x=560, not closer together: DrawFader centers its label
	// under the control regardless of the control's own (narrow) width, so
	// a ~14-character label extends roughly +-49px past the fader's
	// center — placed 100px apart, as this page first had them, that
	// overflow collided with MeterV's label text.
	f.MeterV(340, 30, 24, 90, clampedFrac(m.enc[3]), t.OnColor, t.DarkGray)
	f.Text(340, 132, "MeterV (enc 4)", t.Gray)

	f.Fader(560, 30, 24, 90, module.Knob{Label: "Fader (enc 5)", Value: clampedFrac(m.enc[4]) * 100, Min: 0, Max: 100})
}

// List and HList stack vertically rather than sit side by side: neither
// widget takes an x offset (DrawListRows/DrawListCols always start at
// column 0), so a side-by-side layout — this page's first attempt — drew
// both starting at x=0 and overlapping each other.
func (m *Module) drawLists(f *module.Frame, w, h int, t module.Theme) {
	const rows = 15
	const listMaxY = 90
	cursor := int(clampedFrac(m.enc[5]) * (rows - 1))
	scroll := cursor - 2
	if scroll < 0 {
		scroll = 0
	}
	if scroll > rows-4 {
		scroll = rows - 4
	}
	items := make([]module.ListRow, rows)
	for i := range items {
		items[i] = module.ListRow{Text: fmt.Sprintf("item %d", i+1), Bg: t.Black, TextCol: t.White}
	}
	f.List(module.ListView{Rows: items, Cursor: cursor, Scroll: scroll, Breadcrumb: "List (enc 6)"}, 20, w, 13, listMaxY)

	const cols = 10
	const hlistY = 95
	const hlistH = 40
	visCols := w / 110
	hcursor := int(clampedFrac(m.enc[6]) * (cols - 1))
	hscroll := hcursor - visCols/2
	if hscroll < 0 {
		hscroll = 0
	}
	if hscroll > cols-visCols {
		hscroll = cols - visCols
	}
	hitems := make([]module.ListRow, cols)
	for i := range hitems {
		hitems[i] = module.ListRow{Text: fmt.Sprintf("c%d", i+1), Bg: t.Black, TextCol: t.White}
	}
	f.HList(module.HListView{Cols: hitems, Cursor: hcursor, Scroll: hscroll, Breadcrumb: "HList (enc 7)"}, hlistY, w, hlistH, 110, w)
}

func (m *Module) drawButtons(f *module.Frame, w, h int, t module.Theme) {
	f.Text(10, 40, "press soft buttons 1-3: exclusive group  |  5-6: independent toggles", t.Gray)

	state := func(g *module.ButtonGroup, i int) module.SoftButtonState {
		if g.IsSelected(i) {
			return widgets.SoftOn
		}
		return widgets.SoftNeutral
	}
	buttons := [8]module.SoftButton{
		{Label: "1/16", State: state(m.quantize, 0), Group: 1},
		{Label: "1/8", State: state(m.quantize, 1), Group: 1},
		{Label: "1/4", State: state(m.quantize, 2), Group: 1},
		{},
		{Label: "MUTE", State: state(m.toggles, 4), Group: 2},
		{Label: "SOLO", State: state(m.toggles, 5), Group: 2},
	}
	f.BotStrip(h-18, w, w/8, 18, buttons, "")
}

func (m *Module) drawPadGrid(f *module.Frame, w, h int, t module.Theme) {
	f.Text(300, 40, "press pads to toggle cells (also lights their own LED)", t.Gray)
	colors := make([][]color.NRGBA, 8)
	for row := range 8 {
		colors[row] = make([]color.NRGBA, 8)
		for col := range 8 {
			c := t.DarkGray
			if m.pads[row][col] {
				c = t.OnColor
			}
			colors[row][col] = c
		}
	}
	f.PadGrid(10, 28, 14, colors)
}

func (m *Module) drawFonts(f *module.Frame, w, h int, t module.Theme) {
	weights := [4]module.Weight{module.Regular, module.Bold, module.Italic, module.BoldItalic}
	names := [4]string{"Regular", "Bold", "Italic", "BoldItalic"}
	idx := int(clampedFrac(m.enc[7]) * 3.999)
	size := 14 + clampedFrac(m.enc[0])*20 // enc 1: 14-34pt

	f.Text(10, 40, "enc 8: weight   enc 1: size", t.Gray)
	f.StyledText(10, 80, fmt.Sprintf("%s @ %.0fpt", names[idx], size), t.White, weights[idx], size)
	f.TextScaled(10, 130, "TextScaled 2x", t.White, 2)
}

// drawHelvetica is a static grid (no encoder input) covering every
// StyledText combination in one glance — weight x size x color — so the
// Helvetica Neue swap can be eyeballed on real hardware without dialing
// each case in one at a time.
func (m *Module) drawHelvetica(f *module.Frame, w, h int, t module.Theme) {
	weights := [4]module.Weight{module.Regular, module.Bold, module.Italic, module.BoldItalic}
	names := [4]string{"Regular", "Bold", "Italic", "BoldItalic"}
	sizes := [2]float64{14, 22}
	colors := [4]color.NRGBA{t.White, t.DirColor, t.OnColor, t.Gray}

	rowH := (h - 20 - 18) / len(weights)
	for row, wgt := range weights {
		y := 20 + row*rowH
		x := 10
		for _, size := range sizes {
			s := fmt.Sprintf("%s %.0fpt", names[row], size)
			f.StyledText(x, y+rowH-6, s, colors[row], wgt, size)
			x += int(size)*len(s)/2 + 40
		}
	}
}

func (m *Module) drawEnvelope(f *module.Frame, w, h int, t module.Theme) {
	points := make([]float64, 5)
	for i := range points {
		points[i] = clampedFrac(m.enc[i])
	}
	f.Text(10, 40, "enc 1-5: envelope points", t.Gray)
	f.Envelope(40, 50, w-80, 80, points, t.OnColor)
}
