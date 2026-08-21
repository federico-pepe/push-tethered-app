package renderframe

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

func newDst() *image.NRGBA { return image.NewNRGBA(image.Rect(0, 0, 960, 160)) }

// TestUnknownOpIsSkippedAndCounted is the load-bearing test for the whole
// extensible-op design. Ops are name-plus-payload precisely so a module built
// against a newer core/gfx can run on an older host — which is only true if an
// unrecognised op costs one element and nothing else. If this ever becomes a
// panic or an aborted frame, the open op set stops being safe.
func TestUnknownOpIsSkippedAndCounted(t *testing.T) {
	f := module.NewFrame(960, 160)
	f.Rect(0, 0, 10, 10, color.NRGBA{255, 0, 0, 255})
	// Forge an op no host knows. Modules cannot do this through the typed
	// constructors, but a future or third-party module absolutely can.
	injectRaw(t, f, "no-such-widget-from-the-future", `{"x":1}`)
	f.Rect(20, 0, 10, 10, color.NRGBA{0, 255, 0, 255})

	dst := newDst()
	st := Render(f, dst, widgets.Default)

	if st.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", st.Unknown)
	}
	if st.Failed != 0 {
		t.Errorf("Failed = %d, want 0", st.Failed)
	}
	// Crucially: the ops on *both* sides of the unknown one still drew.
	if got := dst.NRGBAAt(1, 1); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("op before the unknown one did not draw: %+v", got)
	}
	if got := dst.NRGBAAt(21, 1); got != (color.NRGBA{0, 255, 0, 255}) {
		t.Errorf("op after the unknown one did not draw: %+v", got)
	}
}

// TestMalformedParamsAreCountedNotFatal covers the other half: a known op whose
// payload is garbage. Same requirement — skip, count, keep going.
func TestMalformedParamsAreCountedNotFatal(t *testing.T) {
	f := module.NewFrame(960, 160)
	injectRaw(t, f, "rect", `{"x":"not a number"}`)
	f.Rect(0, 0, 10, 10, color.NRGBA{0, 0, 255, 255})

	dst := newDst()
	st := Render(f, dst, widgets.Default)

	if st.Failed != 1 {
		t.Errorf("Failed = %d, want 1", st.Failed)
	}
	if st.Unknown != 0 {
		t.Errorf("Unknown = %d, want 0", st.Unknown)
	}
	if got := dst.NRGBAAt(1, 1); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Errorf("op after the malformed one did not draw: %+v", got)
	}
}

// TestOffCanvasOpsAreHarmless documents why the host does not implement clipping
// of its own: gfx.FillRect goes through draw.Draw and text.Draw through
// font.Drawer, and both already clip to the destination. If an upstream change
// ever broke that, this test would panic rather than let it reach hardware.
func TestOffCanvasOpsAreHarmless(t *testing.T) {
	f := module.NewFrame(960, 160)
	f.Rect(-500, -500, 100, 100, color.NRGBA{255, 0, 0, 255})
	f.Rect(2000, 2000, 100, 100, color.NRGBA{255, 0, 0, 255})
	f.Rect(940, 150, 100, 100, color.NRGBA{0, 255, 0, 255}) // straddles the edge
	f.Text(-50, -50, "offscreen", color.NRGBA{255, 255, 255, 255})
	f.Text(950, 300, "offscreen", color.NRGBA{255, 255, 255, 255})

	dst := newDst()
	st := Render(f, dst, widgets.Default)

	if st.Unknown != 0 || st.Failed != 0 {
		t.Errorf("off-canvas ops should render cleanly, got %+v", st)
	}
	// The straddling rect must still paint the part that is on-canvas.
	if got := dst.NRGBAAt(950, 155); got != (color.NRGBA{0, 255, 0, 255}) {
		t.Errorf("straddling rect did not draw its visible part: %+v", got)
	}
}

// TestNonASCIIIsSanitised pins the ASCII rule. basicfont.Face7x13 has no glyph
// beyond ASCII and draws a missing-glyph box instead, and per feasibility §9.4
// that kind of bug is invisible in logs that report a healthy frame rate. The
// host therefore substitutes rather than trusting every module author.
func TestNonASCIIIsSanitised(t *testing.T) {
	// An em-dash and an accent must render exactly as '?' would.
	gotImg := renderText(t, "a—bé")
	wantImg := renderText(t, "a?b?")
	if !bytes.Equal(gotImg.Pix, wantImg.Pix) {
		t.Error("non-ASCII text did not render identically to its '?' substitution")
	}
}

// TestEllipsisBecomesPeriod covers the specific case that actually occurs.
// Upstream's own text.Truncate appends U+2026 when it cuts a string, so the most
// likely source of non-ASCII is a helper modules are encouraged to use. A '?'
// there would read as an error rather than a truncation.
func TestEllipsisBecomesPeriod(t *testing.T) {
	gotImg := renderText(t, "cut…")
	wantImg := renderText(t, "cut.")
	if !bytes.Equal(gotImg.Pix, wantImg.Pix) {
		t.Error("truncation ellipsis did not render as '.'")
	}
	// And it must not render as '?', which is what a naive substitution gives.
	notWant := renderText(t, "cut?")
	if bytes.Equal(gotImg.Pix, notWant.Pix) {
		t.Error("truncation ellipsis rendered as '?' instead of '.'")
	}
}

// TestASCIIIsUntouched makes sure the sanitiser is a no-op on the normal path —
// it runs on every string of every frame.
func TestASCIIIsUntouched(t *testing.T) {
	const s = "pad 99 (8,8) ch1 vel 127 +-*/[]{}<>"
	if got := asciiOnly(s); got != s {
		t.Errorf("asciiOnly(%q) = %q, want unchanged", s, got)
	}
	// Control characters are not printable either.
	if got := asciiOnly("a\tb\nc"); got != "a?b?c" {
		t.Errorf("asciiOnly on control chars = %q, want \"a?b?c\"", got)
	}
}

// TestSanitisedInsideWidgets checks the substitution reaches text nested inside
// widget parameters, not just the bare text op — a list row or a soft-button
// label is just as capable of carrying an em-dash.
func TestSanitisedInsideWidgets(t *testing.T) {
	dst := newDst()
	f := module.NewFrame(960, 160)
	f.List(module.ListView{
		Rows:       []module.ListRow{{Text: "row—one"}},
		Breadcrumb: "crumb—bar",
	}, 0, 960, 14, 140)
	f.BotStrip(140, 960, 120, 18, [8]module.SoftButton{{Label: "a—b"}}, "hint—")
	f.KVRows(0, 960, 14, 100, 140, []module.KVRow{{Label: "l—", Value: "v—"}})
	f.Header(0, 960, 18, "head—er")
	f.Breadcrumb(0, 960, "crumb—two", "stat—us")
	f.HList(module.HListView{
		Cols:       []module.ListRow{{Text: "col—one"}},
		Breadcrumb: "hcrumb—bar",
	}, 0, 960, 40, 120, 960)

	if st := Render(f, dst, widgets.Default); st.Unknown != 0 || st.Failed != 0 {
		t.Fatalf("widget ops failed to render: %+v", st)
	}

	want := newDst()
	wf := module.NewFrame(960, 160)
	wf.List(module.ListView{
		Rows:       []module.ListRow{{Text: "row?one"}},
		Breadcrumb: "crumb?bar",
	}, 0, 960, 14, 140)
	wf.BotStrip(140, 960, 120, 18, [8]module.SoftButton{{Label: "a?b"}}, "hint?")
	wf.KVRows(0, 960, 14, 100, 140, []module.KVRow{{Label: "l?", Value: "v?"}})
	wf.Header(0, 960, 18, "head?er")
	wf.Breadcrumb(0, 960, "crumb?two", "stat?us")
	wf.HList(module.HListView{
		Cols:       []module.ListRow{{Text: "col?one"}},
		Breadcrumb: "hcrumb?bar",
	}, 0, 960, 40, 120, 960)
	Render(wf, want, widgets.Default)

	if !bytes.Equal(dst.Pix, want.Pix) {
		t.Error("non-ASCII inside widget parameters was not sanitised")
	}
}

// TestImageOpBlits covers the escape hatch, including a bad ref.
func TestImageOpBlits(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range src.Pix {
		src.Pix[i] = 0xFF
	}

	f := module.NewFrame(960, 160)
	f.Image(10, 20, src)
	dst := newDst()
	if st := Render(f, dst, widgets.Default); st.Failed != 0 {
		t.Fatalf("image op failed: %+v", st)
	}
	if got := dst.NRGBAAt(11, 21); got != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("image did not blit at the offset: %+v", got)
	}
	if got := dst.NRGBAAt(0, 0); got != (color.NRGBA{}) {
		t.Errorf("image blitted outside its rect: %+v", got)
	}

	// A dangling ref must be counted, not panic.
	f2 := module.NewFrame(960, 160)
	injectRaw(t, f2, "image", `{"x":0,"y":0,"ref":7}`)
	if st := Render(f2, newDst(), widgets.Default); st.Failed != 1 {
		t.Errorf("dangling image ref: Failed = %d, want 1", st.Failed)
	}
}

// TestSupportedOpsCoversEveryConstructor makes sure no Frame method can emit an
// op the default host cannot render. A constructor added without a handler would
// otherwise draw nothing and only show up on hardware.
func TestSupportedOpsCoversEveryConstructor(t *testing.T) {
	f := module.NewFrame(960, 160)
	f.Rect(0, 0, 1, 1, color.NRGBA{})
	f.Text(0, 0, "x", color.NRGBA{})
	f.Border(0, 0, 1, 1, color.NRGBA{})
	f.HLine(0, 0, 1, color.NRGBA{})
	f.VLine(0, 0, 1, color.NRGBA{})
	f.Meter(0, 0, 1, 1, 0, color.NRGBA{}, color.NRGBA{})
	f.Arc(0, 0, 1, 0, color.NRGBA{})
	f.Header(0, 1, 1, "x")
	f.Breadcrumb(0, 1, "x", "")
	f.KVRows(0, 1, 1, 1, 1, nil)
	f.List(module.ListView{}, 0, 1, 1, 1)
	f.HList(module.HListView{}, 0, 1, 1, 1, 1)
	f.BotStrip(0, 1, 1, 1, [8]module.SoftButton{}, "")
	f.Image(0, 0, image.NewNRGBA(image.Rect(0, 0, 1, 1)))

	supported := map[string]bool{}
	for _, k := range SupportedOps() {
		supported[k] = true
	}
	for _, op := range f.Ops() {
		if !supported[op.Kind] {
			t.Errorf("Frame can emit op %q but the host has no renderer for it", op.Kind)
		}
	}

	if st := Render(f, newDst(), widgets.Default); st.Unknown != 0 {
		t.Errorf("Render reported %d unknown ops for built-in constructors", st.Unknown)
	}
}

// TestRegisterOpExtends proves the extension point works the way the design
// claims: registering a handler is all it takes to support a new op.
func TestRegisterOpExtends(t *testing.T) {
	const kind = "test-only-diagonal"
	called := false
	RegisterOp(kind, func(dst *image.NRGBA, _ module.Theme, _ *module.Frame, p json.RawMessage) error {
		var v struct {
			N int `json:"n"`
		}
		if err := json.Unmarshal(p, &v); err != nil {
			return err
		}
		if v.N != 42 {
			return errors.New("params did not arrive")
		}
		called = true
		return nil
	})
	t.Cleanup(func() {
		opsMu.Lock()
		delete(ops, kind)
		opsMu.Unlock()
	})

	f := module.NewFrame(960, 160)
	injectRaw(t, f, kind, `{"n":42}`)
	if st := Render(f, newDst(), widgets.Default); st.Unknown != 0 || st.Failed != 0 {
		t.Fatalf("newly registered op did not render: %+v", st)
	}
	if !called {
		t.Error("registered handler was not invoked")
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func renderText(t *testing.T, s string) *image.NRGBA {
	t.Helper()
	f := module.NewFrame(960, 160)
	f.Text(10, 20, s, color.NRGBA{255, 255, 255, 255})
	dst := newDst()
	if st := Render(f, dst, widgets.Default); st.Unknown != 0 || st.Failed != 0 {
		t.Fatalf("text op failed to render: %+v", st)
	}
	return dst
}

// injectRaw appends an op that the typed constructors cannot produce. Used to
// simulate a module from the future, or a malformed payload off a wire.
func injectRaw(t *testing.T, f *module.Frame, kind, params string) {
	t.Helper()
	if !json.Valid([]byte(params)) {
		t.Fatalf("injectRaw: %q is not valid JSON", params)
	}
	f.AppendRaw(kind, json.RawMessage(params))
}
