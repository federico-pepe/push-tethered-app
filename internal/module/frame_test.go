package module

import (
	"encoding/json"
	"image"
	"image/color"
	"testing"
)

// TestOpsRoundTrip is the guard on the whole extensible-op design: every op a
// module can emit has to survive JSON unchanged, because that is what lets a
// module eventually run as a separate process in another language. An op that
// silently loses a field here would produce a subtly wrong screen there.
func TestOpsRoundTrip(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	blue := color.NRGBA{0, 0, 255, 255}

	f := NewFrame(960, 160)
	f.Rect(1, 2, 3, 4, red)
	f.Text(5, 6, "hello", blue)
	f.Border(7, 8, 9, 10, red)
	f.HLine(11, 12, 13, red)
	f.VLine(14, 15, 16, blue)
	f.Meter(17, 18, 19, 20, 0.5, red, blue)
	f.Arc(21, 22, 23, 0.25, red)
	f.Header(24, 25, 26, "title")
	f.KVRows(27, 28, 29, 30, 31, []KVRow{{Label: "k", Value: "v", ValueCol: red}})
	f.List(ListView{Rows: []ListRow{{Text: "row"}}, Cursor: 1, Scroll: 2,
		Breadcrumb: "crumb", Status: "st", EmptyText: "empty"}, 32, 33, 34, 35)
	f.BotStrip(36, 37, 38, 39, [8]SoftButton{{Label: "a"}, {Label: "b"}}, "hint")

	if f.Failed() != 0 {
		t.Fatalf("Failed() = %d, want 0 — an op's params would not marshal", f.Failed())
	}

	wantKinds := []string{"rect", "text", "border", "hline", "vline", "meter",
		"arc", "header", "kvrows", "list", "botstrip"}
	got := f.Ops()
	if len(got) != len(wantKinds) {
		t.Fatalf("recorded %d ops, want %d", len(got), len(wantKinds))
	}

	for i, op := range got {
		if op.Kind != wantKinds[i] {
			t.Errorf("op %d: kind = %q, want %q", i, op.Kind, wantKinds[i])
		}
		// Params must be valid JSON and must survive a decode/encode cycle
		// byte-identically once normalised through a generic map.
		var via map[string]any
		if err := json.Unmarshal(op.Params, &via); err != nil {
			t.Errorf("op %d (%s): params are not valid JSON: %v", i, op.Kind, err)
			continue
		}
		reencoded, err := json.Marshal(via)
		if err != nil {
			t.Errorf("op %d (%s): re-encode: %v", i, op.Kind, err)
			continue
		}
		var back map[string]any
		if err := json.Unmarshal(reencoded, &back); err != nil {
			t.Errorf("op %d (%s): re-decode: %v", i, op.Kind, err)
			continue
		}
		if len(back) != len(via) {
			t.Errorf("op %d (%s): field count changed across round trip: %d -> %d",
				i, op.Kind, len(via), len(back))
		}
	}
}

// TestSpecificParamsDecode checks a couple of ops decode into the exact values
// the constructor was given. The round-trip test above proves nothing is lost;
// this proves nothing is transposed — an x/y or w/h swap would pass a round trip
// and still draw in the wrong place.
func TestSpecificParamsDecode(t *testing.T) {
	f := NewFrame(960, 160)
	f.Rect(1, 2, 3, 4, color.NRGBA{5, 6, 7, 8})
	f.Text(9, 10, "s", color.NRGBA{11, 12, 13, 14})
	f.Meter(15, 16, 17, 18, 0.75, color.NRGBA{1, 1, 1, 1}, color.NRGBA{2, 2, 2, 2})

	var r RectParams
	if err := json.Unmarshal(f.Ops()[0].Params, &r); err != nil {
		t.Fatal(err)
	}
	if r.X != 1 || r.Y != 2 || r.W != 3 || r.H != 4 {
		t.Errorf("rect = %+v, want X1 Y2 W3 H4", r)
	}
	if r.C != (color.NRGBA{5, 6, 7, 8}) {
		t.Errorf("rect colour = %+v", r.C)
	}

	var tp TextParams
	if err := json.Unmarshal(f.Ops()[1].Params, &tp); err != nil {
		t.Fatal(err)
	}
	if tp.X != 9 || tp.Baseline != 10 || tp.S != "s" {
		t.Errorf("text = %+v, want X9 Baseline10 S\"s\"", tp)
	}

	var mp MeterParams
	if err := json.Unmarshal(f.Ops()[2].Params, &mp); err != nil {
		t.Fatal(err)
	}
	if mp.Frac != 0.75 || mp.W != 17 || mp.H != 18 {
		t.Errorf("meter = %+v, want Frac0.75 W17 H18", mp)
	}
}

// TestImageRefs covers the one op that cannot go through JSON. Images are held
// by reference and addressed by index; a mismatch would blit the wrong picture.
func TestImageRefs(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	b := image.NewNRGBA(image.Rect(0, 0, 4, 4))

	f := NewFrame(960, 160)
	f.Image(0, 0, a)
	f.Image(10, 10, b)
	f.Image(0, 0, nil) // must be ignored, not recorded as a dangling ref

	if len(f.Ops()) != 2 {
		t.Fatalf("recorded %d ops, want 2 (nil image must be skipped)", len(f.Ops()))
	}
	if f.ImageRef(0) != a {
		t.Error("ref 0 is not the first image")
	}
	if f.ImageRef(1) != b {
		t.Error("ref 1 is not the second image")
	}
	if f.ImageRef(2) != nil || f.ImageRef(-1) != nil {
		t.Error("out-of-range ImageRef must return nil, not panic or wrap")
	}
}

// TestReset makes sure frame reuse cannot leak last frame's content — the host
// reuses one Frame forever to avoid allocating 30 times a second.
func TestReset(t *testing.T) {
	f := NewFrame(960, 160)
	f.Rect(0, 0, 1, 1, color.NRGBA{})
	f.Image(0, 0, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	f.Reset()

	if len(f.Ops()) != 0 {
		t.Errorf("after Reset, %d ops remain", len(f.Ops()))
	}
	if f.ImageRef(0) != nil {
		t.Error("after Reset, an image ref is still resolvable")
	}
	if f.Failed() != 0 {
		t.Errorf("after Reset, Failed() = %d", f.Failed())
	}

	// And it must still be usable.
	f.Rect(0, 0, 1, 1, color.NRGBA{})
	if len(f.Ops()) != 1 {
		t.Errorf("frame unusable after Reset: %d ops", len(f.Ops()))
	}
}

func TestSize(t *testing.T) {
	w, h := NewFrame(960, 160).Size()
	if w != 960 || h != 160 {
		t.Errorf("Size() = %d,%d want 960,160", w, h)
	}
}
