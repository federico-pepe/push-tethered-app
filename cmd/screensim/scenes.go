package main

import (
	"fmt"
	"image"
	"image/color"

	"github.com/federico-pepe/ableton-push-hack/core/gfx"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/layout"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// frameScenes build a *module.Frame the way a module's Draw would, so
// renderframe.Render exercises the exact path a real run takes.
var frameScenes = map[string]func(*module.Frame){
	"kitchen-sink":  sceneKitchenSink,
	"botstrip":      sceneBotStrip,
	"list":          sceneList,
	"breadcrumb":    sceneBreadcrumb,
	"hlist":         sceneHList,
	"button-groups": sceneButtonGroups,
	"controls":      sceneControls,
	"padgrid":       scenePadGrid,
	"text-scale":    sceneTextScale,
}

// drawScenes draw straight onto the canvas with core/gfx and
// core/gfx/widgets, for prototyping a widget that has no Frame op yet.
var drawScenes = map[string]func(*image.NRGBA){
	"meter-arc":   sceneMeterArc,
	"grid-splits": sceneGridSplits,
	"meterv":      sceneMeterV,
}

func sceneKitchenSink(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	f.Header(0, 960, 18, "screensim kitchen sink")
	f.KVRows(20, 300, 14, 100, 90, []module.KVRow{
		{Label: "tempo", Value: "120.0"},
		{Label: "mode", Value: "session"},
	})
	f.Meter(320, 30, 200, 10, 0.65, widgets.Default.OnColor, widgets.Default.DarkGray)
	f.Arc(700, 70, 40, 0.75, widgets.Default.Select)
	f.BotStrip(140, 960, 120, 20, [8]module.SoftButton{
		{Label: "PLAY", State: widgets.SoftOn},
		{Label: "STOP", State: widgets.SoftOff},
		{Label: "DEL", State: widgets.SoftConfirm},
	}, "hint")
}

func sceneBotStrip(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	var buttons [8]module.SoftButton
	states := []module.SoftButtonState{widgets.SoftNeutral, widgets.SoftOn, widgets.SoftOff, widgets.SoftConfirm}
	for i := range buttons {
		buttons[i] = module.SoftButton{Label: "BTN", State: states[i%len(states)]}
	}
	f.BotStrip(140, 960, 120, 20, buttons, "")
}

// sceneBreadcrumb shows the new standalone Breadcrumb op: a top bar with
// no list under it, plus the same bar showing a status message instead
// (Status overrides Breadcrumb — drawn side by side here for comparison).
func sceneBreadcrumb(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	f.Breadcrumb(0, 960, "root / folder / deep", "")
	f.Breadcrumb(20, 960, "root / folder / deep", "3 files copied") // Status overrides Breadcrumb
}

// sceneHList shows the new horizontal-scroll list: 12 columns, only 8 fit
// at colW=120, so DrawScrollbarH's thumb gutter should be visible along
// the bottom edge — the horizontal counterpart to sceneList's vertical
// DrawScrollbar.
func sceneHList(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	cols := make([]module.ListRow, 0, 12)
	for i := 0; i < 12; i++ {
		cols = append(cols, module.ListRow{Text: "preset"})
	}
	f.HList(module.HListView{
		Cols:       cols,
		Cursor:     3,
		Breadcrumb: "presets (scroll right)",
	}, 0, 960, 140, 120, 960)
}

// sceneButtonGroups shows two clusters in one BotStrip: buttons 0-2 are an
// exclusive (radio) quantize group — only "1/8" is On — and buttons 4-5 are
// an independent mute/solo pair, both On at once. Each group's underline
// color is what DrawBotStrip's grouping cue looks like; the states
// (SoftOn/SoftNeutral) come from a module.ButtonGroup's IsSelected, the way
// a real module would build this array from its own Handle-driven state.
func sceneButtonGroups(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)

	quantize := module.NewButtonGroup(true)
	quantize.Toggle(1) // "1/8" selected

	toggles := module.NewButtonGroup(false)
	toggles.Toggle(4)
	toggles.Toggle(5)

	state := func(g *module.ButtonGroup, i int) module.SoftButtonState {
		if g.IsSelected(i) {
			return widgets.SoftOn
		}
		return widgets.SoftNeutral
	}

	buttons := [8]module.SoftButton{
		{Label: "1/16", State: state(quantize, 0), Group: 1},
		{Label: "1/8", State: state(quantize, 1), Group: 1},
		{Label: "1/4", State: state(quantize, 2), Group: 1},
		{},
		{Label: "MUTE", State: state(toggles, 4), Group: 2},
		{Label: "SOLO", State: state(toggles, 5), Group: 2},
	}
	f.BotStrip(140, 960, 120, 20, buttons, "")
}

// sceneControls shows one of each new basic control: a radial-progress
// knob, a rotary-pointer knob, a fader, and an envelope curve.
func sceneControls(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	f.Knob(100, 80, 40, module.Knob{Label: "CUTOFF", Value: 65, Min: 0, Max: 100})
	f.KnobFull(280, 80, 40, module.Knob{Label: "RESO", Value: 30, Min: 0, Max: 100})
	f.Fader(440, 20, 24, 110, module.Knob{Label: "VOL", Value: 80, Min: 0, Max: 100})
	f.Envelope(560, 20, 360, 110,
		[]float64{0, 1, 0.6, 0.6, 0}, widgets.Default.OnColor)
}

// scenePadGrid shows the shared grid now used by both modules/monitor and
// modules/seq: a diagonal lit from bottom-left, proving row 0 draws lowest.
func scenePadGrid(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	colors := make([][]color.NRGBA, 8)
	for row := 0; row < 8; row++ {
		colors[row] = make([]color.NRGBA, 8)
		for col := 0; col < 8; col++ {
			c := widgets.Default.DarkGray
			if col == row {
				c = widgets.Default.OnColor
			}
			colors[row][col] = c
		}
	}
	f.PadGrid(10, 10, 14, colors)
}

// sceneTextScale shows scale 1/2/3/4 stacked, plus the exact use case that
// raised plans/2026-08-18-frame-text-scale.md: modules/remap's editor
// wanting its value bigger than its label. Settle "which scale factors are
// enough" by eye here instead of speculatively.
func sceneTextScale(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	y := 20
	for scale := 1; scale <= 4; scale++ {
		f.TextScaled(10, y, fmt.Sprintf("scale %dx", scale), widgets.Default.White, scale)
		y += 10 + scale*13
	}

	// The remap use case: label small, value big, both centered under a column.
	f.Text(700, 20, "CUTOFF", widgets.Default.Gray)
	f.TextScaled(690, 45, "64", widgets.Default.White, 2)
}

func sceneList(f *module.Frame) {
	f.Rect(0, 0, 960, 160, widgets.Default.Black)
	rows := make([]module.ListRow, 0, 10)
	for i := 0; i < 10; i++ {
		rows = append(rows, module.ListRow{Text: "item"})
	}
	f.List(module.ListView{
		Rows:       rows,
		Cursor:     2,
		Scroll:     0,
		Breadcrumb: "root / folder",
	}, 0, 960, 14, 160)
}

// sceneGridSplits demonstrates core/gfx/layout: a top bar carved off with
// layout.Content, then a 4+4 split in the row below it and a 6+2 split in
// the row below that, each block filled and labelled with its own column
// span so the widths are visually checkable against the -grid overlay.
func sceneGridSplits(img *image.NRGBA) {
	t := widgets.Default
	gfx.FillRect(img, 0, 0, 960, 160, t.Black)

	content := layout.Content(960, 160, layout.Bars{TopH: 18})
	widgets.DrawHeader(img, t, 0, 960, layout.Bars{TopH: 18}.TopH, "layout: top bar + 4+4 + 6+2")

	rowH := (content.Dy() - 8) / 2
	row1Y := content.Min.Y + 4
	row2Y := row1Y + rowH + 8

	x, w := layout.ColSpan(960, 0, 4)
	gfx.FillRect(img, x, row1Y, w-4, rowH, t.Select)
	text.Draw(img, x+8, row1Y+rowH-6, "4 cols", t.White)

	x, w = layout.ColSpan(960, 4, 4)
	gfx.FillRect(img, x, row1Y, w-4, rowH, t.Accent)
	text.Draw(img, x+8, row1Y+rowH-6, "4 cols", t.White)

	x, w = layout.ColSpan(960, 0, 6)
	gfx.FillRect(img, x, row2Y, w-4, rowH, t.OnColor)
	text.Draw(img, x+8, row2Y+rowH-6, "6 cols", t.Black)

	x, w = layout.ColSpan(960, 6, 2)
	gfx.FillRect(img, x, row2Y, w-4, rowH, t.OffColor)
	text.Draw(img, x+8, row2Y+rowH-6, "2 cols", t.Black)
}

// sceneMeterV shows four vertical meters at different fill levels, side by
// side — the layout an 8-channel level meter row would use.
func sceneMeterV(img *image.NRGBA) {
	t := widgets.Default
	gfx.FillRect(img, 0, 0, 960, 160, t.Black)
	fracs := []float64{0.1, 0.4, 0.75, 1.0}
	for i, frac := range fracs {
		x := 40 + i*80
		widgets.DrawMeterV(img, x, 10, 24, 130, frac, t.OnColor, t.DarkGray)
	}
}

func sceneMeterArc(img *image.NRGBA) {
	gfx.FillRect(img, 0, 0, 960, 160, widgets.Default.Black)
	widgets.DrawMeter(img, 40, 40, 300, 14, 0.4, widgets.Default.OnColor, widgets.Default.DarkGray)
	widgets.DrawArc(img, 700, 80, 50, 0.6, widgets.Default.Select)
	text.Draw(img, 40, 90, "direct-draw prototype, no Frame op yet", color.NRGBA{255, 255, 255, 255})
}
