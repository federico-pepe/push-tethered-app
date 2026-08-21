package main

import (
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
	"kitchen-sink": sceneKitchenSink,
	"botstrip":     sceneBotStrip,
	"list":         sceneList,
	"breadcrumb":   sceneBreadcrumb,
}

// drawScenes draw straight onto the canvas with core/gfx and
// core/gfx/widgets, for prototyping a widget that has no Frame op yet.
var drawScenes = map[string]func(*image.NRGBA){
	"meter-arc":   sceneMeterArc,
	"grid-splits": sceneGridSplits,
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

func sceneMeterArc(img *image.NRGBA) {
	gfx.FillRect(img, 0, 0, 960, 160, widgets.Default.Black)
	widgets.DrawMeter(img, 40, 40, 300, 14, 0.4, widgets.Default.OnColor, widgets.Default.DarkGray)
	widgets.DrawArc(img, 700, 80, 50, 0.6, widgets.Default.Select)
	text.Draw(img, 40, 90, "direct-draw prototype, no Frame op yet", color.NRGBA{255, 255, 255, 255})
}
