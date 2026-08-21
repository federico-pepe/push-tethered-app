package main

import (
	"image"
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

func TestScenesRenderCorrectlySizedNonBlankImages(t *testing.T) {
	for name := range frameScenes {
		checkScene(t, name)
	}
	for name := range drawScenes {
		checkScene(t, name)
	}
}

func checkScene(t *testing.T, name string) {
	t.Helper()
	img, err := render(name)
	if err != nil {
		t.Fatalf("scene %q: %v", name, err)
	}
	if b := img.Bounds(); b.Dx() != push3.VisW || b.Dy() != push3.VisH {
		t.Fatalf("scene %q: got %dx%d, want %dx%d", name, b.Dx(), b.Dy(), push3.VisW, push3.VisH)
	}
	if isBlank(img) {
		t.Fatalf("scene %q: rendered entirely blank", name)
	}
}

func isBlank(img *image.NRGBA) bool {
	for _, p := range img.Pix {
		if p != 0 {
			return false
		}
	}
	return true
}

func TestUnknownSceneErrors(t *testing.T) {
	if _, err := render("no-such-scene"); err == nil {
		t.Fatal("expected an error for an unknown scene")
	}
}
