// Package renderframe draws a module's display list onto an image.
//
// Split out of internal/host so it can be imported by tools that must not
// link gousb/libusb (internal/host pulls that in transitively through
// internal/display) — currently cmd/screensim, potentially any future
// out-of-process or headless renderer.
package renderframe

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sort"
	"strings"
	"sync"

	"github.com/federico-pepe/ableton-push-hack/core/gfx"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// OpFunc renders one display-list op.
//
// It gets the frame as well as the params because a few ops carry data that
// cannot go through JSON — the image op holds an *image.NRGBA by reference.
type OpFunc func(dst *image.NRGBA, t module.Theme, f *module.Frame, params json.RawMessage) error

var (
	opsMu sync.RWMutex
	ops   = map[string]OpFunc{}
)

// RegisterOp adds a renderer for an op kind, replacing any existing one.
//
// This is the extension point for the display list. When core/gfx or
// core/gfx/widgets gains a component, supporting it here is one RegisterOp call
// plus one typed constructor on module.Frame — no ABI change, no version bump,
// and every existing module keeps working.
func RegisterOp(kind string, fn OpFunc) {
	opsMu.Lock()
	defer opsMu.Unlock()
	ops[kind] = fn
}

// SupportedOps lists the registered op kinds, sorted. Modules can query this
// through Host to degrade gracefully on an older host.
func SupportedOps() []string {
	opsMu.RLock()
	defer opsMu.RUnlock()
	kinds := make([]string, 0, len(ops))
	for k := range ops {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// RenderStats reports what happened while rendering a frame. Both counters
// should be zero in normal operation; they exist so a misbehaving module is
// visible in the log rather than silently drawing nothing.
type RenderStats struct {
	Unknown int // ops whose Kind has no registered renderer
	Failed  int // ops whose params would not unmarshal, or whose renderer errored
}

// Render draws a module's display list onto dst.
//
// It never panics on bad input and never aborts a frame part-way: an op that
// cannot be rendered is counted and skipped, so one bad op costs one element
// rather than the whole screen. That is the guarantee the extensible op set
// depends on — a module built against a newer core/gfx must degrade, not die.
//
// Off-canvas coordinates need no special handling here: gfx.FillRect goes
// through draw.Draw and text.Draw through font.Drawer, and both clip to the
// destination bounds. Ops outside the panel are simply invisible.
func Render(f *module.Frame, dst *image.NRGBA, t module.Theme) RenderStats {
	var st RenderStats
	opsMu.RLock()
	defer opsMu.RUnlock()

	for _, op := range f.Ops() {
		fn, ok := ops[op.Kind]
		if !ok {
			st.Unknown++
			continue
		}
		if err := fn(dst, t, f, op.Params); err != nil {
			st.Failed++
		}
	}
	return st
}

// ── color defaulting ────────────────────────────────────────────────────────

// defaultColor returns c, or white if a module left c at its JSON zero
// value (color.NRGBA{}, fully transparent black) — an omitted "c"/"fg"/"bg"
// field should read as "no color chosen yet", not as invisible.
func defaultColor(c color.NRGBA) color.NRGBA {
	if c == (color.NRGBA{}) {
		return widgets.Default.White
	}
	return c
}

// ── ASCII enforcement ──────────────────────────────────────────────────────

// asciiOnly replaces characters basicfont.Face7x13 cannot draw.
//
// The font covers ASCII only; anything else renders as a missing-glyph box, and
// the two bugs in feasibility §9.4 were both invisible in logs that reported
// healthy frame rates. Enforcing it in one place beats trusting every module
// author to remember.
//
// The ellipsis is special-cased to "." rather than "?" because upstream's own
// text.Truncate appends U+2026 when it cuts a string — so the most likely source
// of non-ASCII text is a helper modules are *encouraged* to use, and "?" there
// would look like an error rather than a truncation.
func asciiOnly(s string) string {
	needs := false
	for _, r := range s {
		if r > 0x7E || r < 0x20 {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '…': // … from text.Truncate
			b.WriteByte('.')
		case r >= 0x20 && r <= 0x7E:
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// ── Built-in op renderers ──────────────────────────────────────────────────

func init() {
	RegisterOp("rect", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.RectParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		gfx.FillRect(dst, v.X, v.Y, v.W, v.H, defaultColor(v.C))
		return nil
	})

	RegisterOp("text", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.TextParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		scale := v.Scale
		if scale <= 0 {
			scale = 1
		}
		text.DrawScaled(dst, v.X, v.Baseline, scale, asciiOnly(v.S), defaultColor(v.C))
		return nil
	})

	RegisterOp("styledtext", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.StyledTextParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		face, err := text.NewFace(v.Weight, v.Size)
		if err != nil {
			return err
		}
		// asciiOnly first, same defense-in-depth as every other text op —
		// DrawWith's own sanitizing is real but this keeps one place
		// responsible for what a module's raw string becomes on screen.
		text.DrawWith(dst, v.X, v.Baseline, asciiOnly(v.S), defaultColor(v.C), face)
		return nil
	})

	RegisterOp("border", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.RectParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawBorder(dst, v.X, v.Y, v.W, v.H, defaultColor(v.C))
		return nil
	})

	RegisterOp("hline", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.LineParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawHLine(dst, v.X, v.Y, v.N, defaultColor(v.C))
		return nil
	})

	RegisterOp("vline", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.LineParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawVLine(dst, v.X, v.Y, v.N, defaultColor(v.C))
		return nil
	})

	RegisterOp("meter", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.MeterParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawMeter(dst, v.X, v.Y, v.W, v.H, v.Frac, defaultColor(v.FG), defaultColor(v.BG))
		return nil
	})

	RegisterOp("meterv", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.MeterVParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawMeterV(dst, v.X, v.Y, v.W, v.H, v.Frac, defaultColor(v.FG), defaultColor(v.BG))
		return nil
	})

	RegisterOp("arc", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.ArcParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawArc(dst, v.CX, v.CY, v.R, v.Frac, defaultColor(v.C))
		return nil
	})

	RegisterOp("padgrid", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.PadGridParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		rows := len(v.Colors)
		if rows == 0 {
			return nil
		}
		cols := len(v.Colors[0])
		widgets.DrawPadGrid(dst, v.X, v.Y, v.Cell, cols, rows, func(col, row int) color.NRGBA {
			if row >= len(v.Colors) || col >= len(v.Colors[row]) {
				return widgets.Default.White
			}
			return defaultColor(v.Colors[row][col])
		})
		return nil
	})

	RegisterOp("knob", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.KnobParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		v.K.Label = asciiOnly(v.K.Label)
		widgets.DrawKnob(dst, t, v.CX, v.CY, v.R, v.K)
		return nil
	})

	RegisterOp("knobfull", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.KnobParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		v.K.Label = asciiOnly(v.K.Label)
		widgets.DrawKnobFull(dst, t, v.CX, v.CY, v.R, v.K)
		return nil
	})

	RegisterOp("knobarc", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.KnobParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		v.K.Label = asciiOnly(v.K.Label)
		widgets.DrawKnobArc(dst, t, v.CX, v.CY, v.R, v.K)
		return nil
	})

	RegisterOp("fader", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.FaderParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		v.K.Label = asciiOnly(v.K.Label)
		widgets.DrawFader(dst, t, v.X, v.Y, v.W, v.H, v.K)
		return nil
	})

	RegisterOp("envelope", func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.EnvelopeParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawEnvelope(dst, v.X, v.Y, v.W, v.H, v.Points, defaultColor(v.C))
		return nil
	})

	RegisterOp("header", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.HeaderParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawHeader(dst, t, v.Y, v.W, v.H, asciiOnly(v.S))
		return nil
	})

	RegisterOp("statusbar", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.StatusBarParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawStatusBar(dst, t, v.Y, v.W, v.H, asciiOnly(v.S), v.IsError)
		return nil
	})

	RegisterOp("breadcrumb", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.BreadcrumbParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		widgets.DrawBreadcrumbBar(dst, t, v.Y, v.W, asciiOnly(v.Breadcrumb), asciiOnly(v.Status))
		return nil
	})

	RegisterOp("kvrows", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.KVRowsParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		for i := range v.Rows {
			v.Rows[i].Label = asciiOnly(v.Rows[i].Label)
			v.Rows[i].Value = asciiOnly(v.Rows[i].Value)
		}
		widgets.DrawKVRows(dst, t, v.Y, v.W, v.RowH, v.LabelW, v.MaxY, v.Rows)
		return nil
	})

	RegisterOp("list", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.ListParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		v.View.Breadcrumb = asciiOnly(v.View.Breadcrumb)
		v.View.Status = asciiOnly(v.View.Status)
		v.View.EmptyText = asciiOnly(v.View.EmptyText)
		for i := range v.View.Rows {
			v.View.Rows[i].Text = asciiOnly(v.View.Rows[i].Text)
		}
		widgets.RenderList(dst, t, v.View, v.Y, v.W, v.RowH, v.MaxY)
		return nil
	})

	RegisterOp("hlist", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.HListParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		v.View.Breadcrumb = asciiOnly(v.View.Breadcrumb)
		v.View.Status = asciiOnly(v.View.Status)
		v.View.EmptyText = asciiOnly(v.View.EmptyText)
		for i := range v.View.Cols {
			v.View.Cols[i].Text = asciiOnly(v.View.Cols[i].Text)
		}
		widgets.RenderListH(dst, t, v.View, v.Y, v.W, v.H, v.ColW, v.MaxX)
		return nil
	})

	RegisterOp("botstrip", func(dst *image.NRGBA, t module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v module.BotStripParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		for i := range v.Buttons {
			v.Buttons[i].Label = asciiOnly(v.Buttons[i].Label)
		}
		widgets.DrawBotStrip(dst, t, v.Y, v.W, v.ColW, v.H, v.Buttons, asciiOnly(v.Hint))
		return nil
	})

	RegisterOp("image", func(dst *image.NRGBA, _ module.Theme, f *module.Frame, p json.RawMessage) error {
		var v module.ImageParams
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		src := f.ImageRef(v.Ref)
		if src == nil {
			return fmt.Errorf("image op: no image at ref %d", v.Ref)
		}
		// draw.Src rather than gfx.DrawIcon: DrawIcon alpha-keys, which is right
		// for icons but wrong for a visualiser blitting a full opaque frame.
		r := src.Bounds().Add(image.Pt(v.X, v.Y))
		draw.Draw(dst, r, src, src.Bounds().Min, draw.Src)
		return nil
	})
}
