// Package mirror serves the frames a Runtime renders as a live MJPEG stream
// over local HTTP, so the exact contents of the Push screen can be watched in
// a browser tab (or embedded as a plain <img>) instead of only on the
// physical panel.
//
// Like internal/capture, it taps the render output rather than the USB
// write, so it costs no extra USB traffic and cannot disturb what the panel
// shows. Unlike capture, a Hub with no connected client does no encoding
// work at all — Frame returns immediately when nobody is watching.
package mirror

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"sync"
)

// jpegQuality trades a little detail for a lot of bandwidth: the panel UI is
// mostly flat colour, so this still looks clean while keeping frames small
// enough for 30fps over loopback HTTP.
const jpegQuality = 80

// Hub receives every rendered frame and fans it out to any number of HTTP
// clients as an MJPEG (multipart/x-mixed-replace) stream.
type Hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

// NewHub returns an idle Hub, ready to be wired into a Runtime and served.
func NewHub() *Hub {
	return &Hub{subs: make(map[chan []byte]struct{})}
}

// Frame encodes and broadcasts img to every connected client. Must not
// retain img past the call — same convention as capture.Recorder.Frame.
// A no-op, before any encoding happens, when there are no clients.
func (h *Hub) Frame(img *image.NRGBA) {
	h.mu.Lock()
	if len(h.subs) == 0 {
		h.mu.Unlock()
		return
	}
	subs := make([]chan []byte, 0, len(h.subs))
	for c := range h.subs {
		subs = append(subs, c)
	}
	h.mu.Unlock()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return
	}
	frame := buf.Bytes()
	for _, c := range subs {
		select {
		case c <- frame:
		default:
			// Slow client — drop this frame for it rather than block the render loop.
		}
	}
}

const boundary = "pushmirror"

// ServeHTTP streams frames as MJPEG, one multipart section per Frame call,
// until the client disconnects.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ch := make(chan []byte, 2)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	// Flush the headers immediately: with nothing written yet, Go's server
	// buffers them until enough body data accumulates, which for a stream
	// that only writes once a frame arrives means a client's request never
	// even sees a response until the first Frame call — confirmed via a
	// hung httptest client while developing this.
	flusher, canFlush := w.(http.Flusher)
	if canFlush {
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case frame := <-ch:
			if _, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", boundary, len(frame)); err != nil {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, "\r\n"); err != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
	}
}
