package mirror

import (
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func solid(c byte) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for i := range img.Pix {
		img.Pix[i] = c
	}
	return img
}

func TestIdleHubDoesNotEncode(t *testing.T) {
	h := NewHub()
	h.Frame(solid(10))
	if h.ver != 0 {
		t.Fatalf("encoded with no viewer: ver=%d", h.ver)
	}
}

func TestFrameDedupeAndVersion(t *testing.T) {
	h := NewHub()
	h.lastPoll.Store(time.Now().UnixNano())
	h.Frame(solid(10))
	h.Frame(solid(10))
	if h.ver != 1 {
		t.Fatalf("same picture bumped version: %d", h.ver)
	}
	h.Frame(solid(20))
	if h.ver != 2 {
		t.Fatalf("new picture kept version: %d", h.ver)
	}
}

func TestServeFrameUpscaledAnd204(t *testing.T) {
	h := NewHub()
	h.lastPoll.Store(time.Now().UnixNano())
	h.Frame(solid(10))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/screen?frame=1&since=0", nil))
	if rec.Code != 200 || rec.Header().Get("X-Mirror-Seq") != "1" {
		t.Fatalf("code=%d seq=%q", rec.Code, rec.Header().Get("X-Mirror-Seq"))
	}
	img, err := png.Decode(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 4*scale || b.Dy() != 2*scale {
		t.Fatalf("size %v", b)
	}
}

func TestServeFrameWakesOnNewPicture(t *testing.T) {
	h := NewHub()
	h.lastPoll.Store(time.Now().UnixNano())
	h.Frame(solid(10))
	go func() {
		time.Sleep(50 * time.Millisecond)
		h.Frame(solid(20))
	}()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/screen?frame=1&since=1", nil))
	if rec.Code != 200 || rec.Header().Get("X-Mirror-Seq") != "2" {
		t.Fatalf("code=%d seq=%q", rec.Code, rec.Header().Get("X-Mirror-Seq"))
	}
}

func TestServePage(t *testing.T) {
	h := NewHub()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/screen", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("code=%d type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !h.watched() {
		t.Fatal("page request should count as a viewer")
	}
}
