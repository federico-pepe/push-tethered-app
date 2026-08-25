// Package padpointer proves a pad-grid-driven pointer, two ways:
//
//   - Page 1 (menu): holding a pad moves a cursor onto an 8-item on-screen
//     menu using the pad's row only, and pressing firmly enough (Channel
//     Pressure) "clicks" (toggles) the highlighted item.
//   - Page 2 (crosshair): the full 8x8 grid positions a crosshair anywhere
//     on screen (col -> x, row -> y) on touch, and pressing firmly enough
//     (Channel Pressure, same threshold as the menu page's click) triggers
//     a short expanding-ring animation to confirm a firm press was
//     detected — a light touch just moves the crosshair.
//
// D-Pad left/right switches pages, same convention as modules/uidemo.
//
// Both pages work off the pad grid's coarse coordinate space — one screen
// position per pad, no finer resolution implied. Push does not expose
// sub-pad XY position over MIDI (confirmed 2026-08-25: Channel Pressure
// exists and is genuinely continuous, but MPE per-note slide/bend, which
// would give finer positioning, needs an undocumented Ableton vendor SysEx
// handshake this app does not send — see docs/protocol/live-handshake.md).
// This module works the same way on Push 2 and Push 3 as a result: no
// device branch needed, since neither currently exposes anything beyond one
// note + channel pressure per gesture.
package padpointer

import (
	"fmt"
	"image/color"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// items is the fixed demo menu — one per pad row, so no scrolling is needed.
var items = [8]string{
	"Item 1", "Item 2", "Item 3", "Item 4",
	"Item 5", "Item 6", "Item 7", "Item 8",
}

// clickThreshold is the Channel Pressure value (0-127) a hold must reach to
// count as a click rather than a hover, on the menu page. Chosen from live
// capture on 2026-08-25: a light touch stayed under 30, a deliberate firm
// press passed 60 comfortably.
const clickThreshold = 60

// numPages and page names, same pattern as modules/uidemo.
const numPages = 2

var pageNames = [numPages]string{"menu", "crosshair"}

// animFrames is how long the crosshair page's press animation runs, in
// Draw calls — short enough to read as a pulse, not a lingering state.
const animFrames = 10

type Module struct {
	host module.Host
	page int

	// Menu page state.
	cursor   int  // 0-7, index into items — which row is pointed at
	holding  bool // is a pad currently pressed
	heldNote byte
	checked  [8]bool
	lastMsg  string

	// Crosshair page state.
	haveCursor     bool // false until the first pad press, so nothing draws at (0,0) by default
	cursorCol      int
	cursorRow      int
	crosshairHold  bool // is a pad currently pressed on the crosshair page
	crosshairNote  byte
	crosshairFired bool // this hold already triggered the animation once
	animFrame      int  // counts up from 0 while animating; animFrames means "not animating"
}

func New() *Module { return &Module{cursor: -1, animFrame: animFrames} }

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:          "padpointer",
		Name:        "Pad Pointer",
		Author:      "Federico Pepe",
		Version:     "0.2.0",
		Description: "Pad-grid pointer: menu page (row + pressure-click) and crosshair page (full grid + press animation)",
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	return nil
}

func (m *Module) Close() error { return nil }

// rowToItem maps a pad row (0 = bottom) to a menu index (0 = top of the
// on-screen list), so pressing the physically top row selects the visually
// top item.
func rowToItem(row int) int { return 7 - row }

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

	case module.Pad:
		if m.page == 1 {
			m.handleCrosshairPad(e)
			return
		}
		m.handleMenuPad(e)

	case module.Expression:
		if e.Kind != "pressure" {
			return
		}
		switch m.page {
		case 0:
			m.handleMenuPressure(e.Value)
		case 1:
			m.handleCrosshairPressure(e.Value)
		}
	}
}

func (m *Module) handleMenuPressure(value int) {
	if !m.holding || value < clickThreshold || m.cursor < 0 {
		return
	}
	m.checked[m.cursor] = !m.checked[m.cursor]
	state := "unchecked"
	if m.checked[m.cursor] {
		state = "checked"
	}
	m.lastMsg = fmt.Sprintf("%s: %s", items[m.cursor], state)
	// Require releasing and re-crossing the threshold before this item can
	// toggle again, so holding past the threshold doesn't flicker the state
	// every Expression message.
	m.holding = false
}

func (m *Module) handleCrosshairPressure(value int) {
	if !m.crosshairHold || m.crosshairFired || value < clickThreshold {
		return
	}
	m.animFrame = 0 // (re)start the "press detected" animation
	// One trigger per hold: without this, every Expression message at or
	// above the threshold (there are many, it's high-rate) would restart
	// the animation and it would never finish playing.
	m.crosshairFired = true
}

func (m *Module) handleMenuPad(e module.Pad) {
	if e.Pressed {
		m.cursor = rowToItem(e.Row)
		m.holding = true
		m.heldNote = e.Note
	} else if e.Note == m.heldNote {
		m.holding = false
	}
}

func (m *Module) handleCrosshairPad(e module.Pad) {
	if e.Pressed {
		m.haveCursor = true
		m.cursorCol, m.cursorRow = e.Col, e.Row
		m.crosshairHold = true
		m.crosshairNote = e.Note
		m.crosshairFired = false
	} else if e.Note == m.crosshairNote {
		m.crosshairHold = false
	}
}

func (m *Module) Draw(f *module.Frame) {
	w, h := f.Size()
	t := m.host.Theme()

	f.Rect(0, 0, w, h, t.Black)
	title := fmt.Sprintf("pushapp - padpointer  [%d/%d] %s  (D-Pad left/right to change page)",
		m.page+1, numPages, pageNames[m.page])
	f.Header(0, w, 20, title)

	switch m.page {
	case 0:
		m.drawMenuPage(f, w, h, t)
	case 1:
		m.drawCrosshairPage(f, w, h, t)
	}
}

func (m *Module) drawMenuPage(f *module.Frame, w, h int, t module.Theme) {
	// Hand-drawn rows rather than Frame.List: List always reserves a
	// breadcrumb strip (13px) this module has no use for, and with 8 fixed
	// rows and no scrolling ever needed, a plain Rect+Text loop fits all
	// eight on screen without that overhead.
	const rowH = 15
	top := 22
	for i, name := range items {
		y := top + i*rowH
		if i == m.cursor {
			f.Rect(0, y, w, rowH, t.DarkGray)
		}
		label := "  " + name
		if m.checked[i] {
			label = "x " + name
		}
		f.Text(6, y+rowH-4, label, t.White)
	}

	status := m.lastMsg
	if status == "" {
		status = "press and hold a pad to point, press firmly to select"
	}
	f.StatusBar(h-18, w, 18, status, false)
}

// crosshairXY maps a pad cell to a screen position within the drawable area
// (below the header, above the status bar), col 0-7 -> left-to-right,
// row 0-7 (0 = bottom) -> bottom-to-top, so the crosshair moves the same
// direction on screen as the finger moves on the grid.
func crosshairXY(w, h, col, row int) (x, y int) {
	const top, bottom = 30, 20 // top margin below header, bottom margin above status bar
	usableW, usableH := w-2*top, h-top-bottom
	x = top + col*usableW/7
	y = top + (7-row)*usableH/7
	return x, y
}

// lerpColor blends from a to b as frac goes 0..1, alpha always opaque.
func lerpColor(a, b color.NRGBA, frac float64) color.NRGBA {
	if frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*frac) }
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

func (m *Module) drawCrosshairPage(f *module.Frame, w, h int, t module.Theme) {
	status := "move: touch any pad — press firmly to trigger the animation"
	if !m.haveCursor {
		f.StatusBar(h-18, w, 18, status, false)
		return
	}

	cx, cy := crosshairXY(w, h, m.cursorCol, m.cursorRow)

	const arm = 8
	f.HLine(cx-arm, cy, 2*arm, t.White)
	f.VLine(cx, cy-arm, 2*arm, t.White)

	if m.animFrame < animFrames {
		// Expanding, fading ring: radius grows with each frame, colour
		// fades from OnColor to black so the pulse reads as dying out
		// rather than stopping abruptly. The renderer's Arc doesn't honour
		// a colour's own alpha channel (it always writes fully opaque —
		// see core/gfx/widgets' blendPixel), so the fade is done here by
		// interpolating RGB toward black instead.
		radius := 4 + m.animFrame*4
		frac := 1 - float64(m.animFrame)/float64(animFrames)
		f.Arc(cx, cy, radius, 1.0, lerpColor(t.Black, t.OnColor, frac))
		m.animFrame++
		status = "press detected!"
	}

	f.StatusBar(h-18, w, 18, status, false)
}
