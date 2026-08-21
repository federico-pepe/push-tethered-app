package main

import (
	"image"
	"image/color"

	"github.com/federico-pepe/ableton-push-hack/core/gfx"
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
}

// drawScenes draw straight onto the canvas with core/gfx and
// core/gfx/widgets, for prototyping a widget that has no Frame op yet.
var drawScenes = map[string]func(*image.NRGBA){
	"meter-arc": sceneMeterArc,
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

func sceneMeterArc(img *image.NRGBA) {
	gfx.FillRect(img, 0, 0, 960, 160, widgets.Default.Black)
	widgets.DrawMeter(img, 40, 40, 300, 14, 0.4, widgets.Default.OnColor, widgets.Default.DarkGray)
	widgets.DrawArc(img, 700, 80, 50, 0.6, widgets.Default.Select)
	text.Draw(img, 40, 90, "direct-draw prototype, no Frame op yet", color.NRGBA{255, 255, 255, 255})
}
