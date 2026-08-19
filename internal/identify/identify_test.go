package identify

import (
	"image"
	"image/color"
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

func TestSanitizeLabel(t *testing.T) {
	tests := []struct{ in, want string }{
		{"AB12CD34", "AB12CD34"},
		{"  AB12  ", "AB12"},
		{"AB?CD", "ABCD"},
		{"AB\x00\x1fCD", "ABCD"},
		{"AB\nCD", "ABCD"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := SanitizeLabel(tt.in); got != tt.want {
			t.Errorf("SanitizeLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeLabelTruncatesLongInput(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "X"
	}
	got := SanitizeLabel(long)
	if len(got) > 40 {
		t.Errorf("SanitizeLabel did not truncate: got %d runes", len(got))
	}
}

func newCanvas() *image.NRGBA {
	return image.NewNRGBA(image.Rect(0, 0, push3.VisW, push3.VisH))
}

// The background must alternate between the swatch and black across phases —
// that alternation is the entire mechanism a viewer uses to tell "this screen
// is flashing" from "this screen is stuck".
func TestPaintMarkerAlternatesBackground(t *testing.T) {
	swatch := color.NRGBA{R: 200, G: 50, B: 50, A: 255}
	corner := image.Point{X: 2, Y: 2} // away from the centred label

	even := newCanvas()
	PaintMarker(even, "", swatch, 0)
	odd := newCanvas()
	PaintMarker(odd, "", swatch, 1)

	gotEven := even.NRGBAAt(corner.X, corner.Y)
	gotOdd := odd.NRGBAAt(corner.X, corner.Y)

	if gotEven != swatch {
		t.Errorf("phase 0 corner = %v, want swatch %v", gotEven, swatch)
	}
	black := color.NRGBA{0, 0, 0, 255}
	if gotOdd != black {
		t.Errorf("phase 1 corner = %v, want black %v", gotOdd, black)
	}
}

// The label must stay legible through the blink: whichever colour is in the
// background, the text draws in the other one, never blending into it.
func TestPaintMarkerLabelContrastsBackground(t *testing.T) {
	swatch := color.NRGBA{R: 0, G: 200, B: 0, A: 255}
	black := color.NRGBA{0, 0, 0, 255}

	for _, phase := range []int{0, 1} {
		img := newCanvas()
		PaintMarker(img, "A", swatch, phase)

		wantBG, wantFG := swatch, black
		if phase%2 == 1 {
			wantBG, wantFG = black, swatch
		}

		// Somewhere in the centre row a foreground-coloured pixel must exist
		// (part of the glyph), and the far corner must be background-coloured.
		foundFG := false
		y := push3.VisH / 2
		for x := 0; x < push3.VisW; x++ {
			if img.NRGBAAt(x, y) == wantFG {
				foundFG = true
				break
			}
		}
		if !foundFG {
			t.Errorf("phase %d: no foreground-coloured pixel found on the label row", phase)
		}
		if got := img.NRGBAAt(1, 1); got != wantBG {
			t.Errorf("phase %d: corner = %v, want background %v", phase, got, wantBG)
		}
	}
}

// A label longer than the screen must not panic or produce a negative draw
// position that wraps around.
func TestPaintMarkerHandlesOverlongLabel(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "X"
	}
	img := newCanvas()
	PaintMarker(img, long, color.NRGBA{R: 255, A: 255}, 0)
}

func TestPaintMarkerHandlesEmptyLabel(t *testing.T) {
	img := newCanvas()
	PaintMarker(img, "", color.NRGBA{G: 255, A: 255}, 0)
}
