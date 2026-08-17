package module

import (
	"encoding/json"
	"image"
	"image/color"
)

// Op is one drawing instruction: a name and an opaque payload.
//
// Deliberately not a closed enum. core/gfx and core/gfx/widgets are expected to
// gain components over time, and a Go type switch over a fixed set would mean
// editing this ABI — and breaking modules — every time one lands. With
// name-plus-payload the host renders from a registry, so a new widget is one
// handler plus one constructor and nothing existing changes.
//
// The rules that make that safe:
//   - Kind names are frozen once shipped. Adding is fine, renaming is breaking.
//   - An unknown Kind is skipped and counted by the host, never fatal.
//   - Modules never build Ops by hand; they call the typed methods on Frame.
type Op struct {
	Kind   string          `json:"kind"`
	Params json.RawMessage `json:"params"`
}

// Frame is the display list for one frame. A module fills it in Draw; the host
// renders it and reuses it for the next frame.
//
// Not safe for concurrent use, and it does not need to be: the host calls Draw
// from the same single goroutine that delivers events.
type Frame struct {
	w, h   int
	ops    []Op
	images []*image.NRGBA
	failed int
}

// NewFrame returns a frame sized to the panel.
func NewFrame(w, h int) *Frame { return &Frame{w: w, h: h} }

// Size returns the panel dimensions in pixels. Use it instead of importing
// geometry constants, so a module keeps working if it ever renders somewhere
// else.
func (f *Frame) Size() (w, h int) { return f.w, f.h }

// Ops returns the recorded display list.
func (f *Frame) Ops() []Op { return f.ops }

// ImageRef resolves an image recorded by Image. Used by the renderer.
func (f *Frame) ImageRef(i int) *image.NRGBA {
	if i < 0 || i >= len(f.images) {
		return nil
	}
	return f.images[i]
}

// Failed reports how many ops could not be recorded because their parameters
// would not marshal. Should always be zero; a non-zero value means a bug in a
// Frame method, not in the module.
func (f *Frame) Failed() int { return f.failed }

// Reset empties the frame for reuse, keeping the allocated capacity.
func (f *Frame) Reset() {
	f.ops = f.ops[:0]
	f.images = f.images[:0]
	f.failed = 0
}

// AppendRaw records an op directly, bypassing the typed constructors.
//
// This exists for two callers, not for modules:
//
//   - the out-of-process loader, which receives a display list as JSON and has
//     to rebuild a Frame from it without knowing every op kind;
//   - tests, which need to simulate a module built against a newer core/gfx to
//     check that unknown ops degrade instead of breaking the frame.
//
// Module code should use the typed methods, which cannot produce a malformed op.
func (f *Frame) AppendRaw(kind string, params json.RawMessage) {
	f.ops = append(f.ops, Op{Kind: kind, Params: params})
}

func (f *Frame) add(kind string, params any) {
	b, err := json.Marshal(params)
	if err != nil {
		f.failed++
		return
	}
	f.ops = append(f.ops, Op{Kind: kind, Params: b})
}

// ── Parameter types ────────────────────────────────────────────────────────
//
// Exported so the host's renderer can unmarshal them, and so a future
// out-of-process loader has a documented wire format. Modules should use the
// methods below rather than these directly.

// RectParams is shared by the rect and border ops. Fields are tagged one per
// line rather than grouped: this is a wire format other languages will have to
// write by hand, so it stays boring and explicit.
type RectParams struct {
	X int         `json:"x"`
	Y int         `json:"y"`
	W int         `json:"w"`
	H int         `json:"h"`
	C color.NRGBA `json:"c"`
}

type TextParams struct {
	X        int         `json:"x"`
	Baseline int         `json:"baseline"`
	S        string      `json:"s"`
	C        color.NRGBA `json:"c"`
}

type LineParams struct {
	X int         `json:"x"`
	Y int         `json:"y"`
	N int         `json:"n"` // length: width for hline, height for vline
	C color.NRGBA `json:"c"`
}

type MeterParams struct {
	X    int         `json:"x"`
	Y    int         `json:"y"`
	W    int         `json:"w"`
	H    int         `json:"h"`
	Frac float64     `json:"frac"`
	FG   color.NRGBA `json:"fg"`
	BG   color.NRGBA `json:"bg"`
}

type ArcParams struct {
	CX   int         `json:"cx"`
	CY   int         `json:"cy"`
	R    int         `json:"r"`
	Frac float64     `json:"frac"`
	C    color.NRGBA `json:"c"`
}

type HeaderParams struct {
	Y int    `json:"y"`
	W int    `json:"w"`
	H int    `json:"h"`
	S string `json:"s"`
}

type KVRowsParams struct {
	Y      int     `json:"y"`
	W      int     `json:"w"`
	RowH   int     `json:"row_h"`
	LabelW int     `json:"label_w"`
	MaxY   int     `json:"max_y"`
	Rows   []KVRow `json:"rows"`
}

type ListParams struct {
	View ListView `json:"view"`
	Y    int      `json:"y"`
	W    int      `json:"w"`
	RowH int      `json:"row_h"`
	MaxY int      `json:"max_y"`
}

type BotStripParams struct {
	Y       int           `json:"y"`
	W       int           `json:"w"`
	ColW    int           `json:"col_w"`
	H       int           `json:"h"`
	Buttons [4]SoftButton `json:"buttons"`
	Hint    string        `json:"hint"`
}

type ImageParams struct {
	X   int `json:"x"`
	Y   int `json:"y"`
	Ref int `json:"ref"`
}

// ── Typed constructors ─────────────────────────────────────────────────────

// Rect fills a rectangle.
func (f *Frame) Rect(x, y, w, h int, c color.NRGBA) {
	f.add("rect", RectParams{X: x, Y: y, W: w, H: h, C: c})
}

// Text draws a string. x is the left edge, baseline is the text baseline — not
// the top — because that is what basicfont works in.
//
// ASCII only. The font has no glyph for anything else and renders a box
// instead, so the host replaces non-ASCII before drawing. Do not rely on that
// to look good; write ASCII.
func (f *Frame) Text(x, baseline int, s string, c color.NRGBA) {
	f.add("text", TextParams{X: x, Baseline: baseline, S: s, C: c})
}

// Border strokes a 1px rectangle outline.
func (f *Frame) Border(x, y, w, h int, c color.NRGBA) {
	f.add("border", RectParams{X: x, Y: y, W: w, H: h, C: c})
}

// HLine draws a horizontal line w pixels wide.
func (f *Frame) HLine(x, y, w int, c color.NRGBA) {
	f.add("hline", LineParams{X: x, Y: y, N: w, C: c})
}

// VLine draws a vertical line h pixels tall.
func (f *Frame) VLine(x, y, h int, c color.NRGBA) {
	f.add("vline", LineParams{X: x, Y: y, N: h, C: c})
}

// Meter draws a horizontal bar filled to frac, clamped to 0..1.
func (f *Frame) Meter(x, y, w, h int, frac float64, fg, bg color.NRGBA) {
	f.add("meter", MeterParams{X: x, Y: y, W: w, H: h, Frac: frac, FG: fg, BG: bg})
}

// Arc draws a circular arc of radius r, filled to frac of a full turn.
func (f *Frame) Arc(cx, cy, r int, frac float64, c color.NRGBA) {
	f.add("arc", ArcParams{CX: cx, CY: cy, R: r, Frac: frac, C: c})
}

// Header draws a themed header bar with a title.
func (f *Frame) Header(y, w, h int, s string) {
	f.add("header", HeaderParams{Y: y, W: w, H: h, S: s})
}

// KVRows draws label/value rows, themed.
func (f *Frame) KVRows(y, w, rowH, labelW, maxY int, rows []KVRow) {
	f.add("kvrows", KVRowsParams{Y: y, W: w, RowH: rowH, LabelW: labelW, MaxY: maxY, Rows: rows})
}

// List draws a scrolling list with an optional breadcrumb bar and scrollbar.
// ListView is a per-frame value: the cursor and scroll offset live in the
// module, not in the widget.
//
// Note for later: ListRow.Icon is an *image.NRGBA, which does not survive a
// process boundary. Icons are in-process only until the out-of-process loader
// gains an image-handle mechanism.
func (f *Frame) List(v ListView, y, w, rowH, maxY int) {
	f.add("list", ListParams{View: v, Y: y, W: w, RowH: rowH, MaxY: maxY})
}

// BotStrip draws the four soft buttons under the screen plus a hint.
func (f *Frame) BotStrip(y, w, colW, h int, buttons [4]SoftButton, hint string) {
	f.add("botstrip", BotStripParams{Y: y, W: w, ColW: colW, H: h, Buttons: buttons, Hint: hint})
}

// Image blits an image — the escape hatch for anything the widget set cannot
// express, such as a visualiser drawing its own pixels.
//
// The image is retained by reference until the frame is reset, so do not mutate
// it after passing it in.
func (f *Frame) Image(x, y int, img *image.NRGBA) {
	if img == nil {
		return
	}
	f.images = append(f.images, img)
	f.add("image", ImageParams{X: x, Y: y, Ref: len(f.images) - 1})
}
