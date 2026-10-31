// Package mirror serves the frames a Runtime renders over local HTTP, so the
// exact contents of the Push screen can be watched in a browser tab instead
// of only on the physical panel.
//
// The URL a Hub is mounted at serves a small page. The page long-polls the
// same URL with ?frame=1&since=<version> and draws each PNG it gets into a
// canvas. An MJPEG stream was used before: Chrome showed its newest frame one
// change late on a screen that changes in bursts, and JPEG blurred coloured
// text. Lossless PNG, fetched one picture at a time, has neither problem.
//
// Like internal/capture, it taps the render output rather than the USB
// write, so it costs no extra USB traffic and cannot disturb what the panel
// shows. With no viewer polling, Frame returns at once and does no work.
package mirror

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// scale is the whole-number nearest-neighbour upscale done here (1 = off).
// The page draws the picture at the panel's width in CSS pixels, so a
// Retina display shows it 1:1 and text stays sharp. Scaling in the browser
// looked worse, smooth or pixelated.
const scale = 2

// viewerTTL is how long after its last poll a viewer still counts. A
// long-poll lasts up to waitMax, so this must be longer than that.
const viewerTTL = 3 * time.Second

// waitMax is the longest a poll waits for a new picture before it answers
// 204 and lets the page ask again.
const waitMax = time.Second

// Lossless: JPEG's chroma subsampling blurs coloured text and edges, and a
// flat UI compresses well. BestSpeed keeps the encode cheap.
var pngEnc = png.Encoder{CompressionLevel: png.BestSpeed, BufferPool: &pngPool{}}

// pngPool keeps the encoder's working buffer between frames. Only Frame
// encodes, from one goroutine, so no locking is needed.
type pngPool struct{ b *png.EncoderBuffer }

func (p *pngPool) Get() *png.EncoderBuffer  { b := p.b; p.b = nil; return b }
func (p *pngPool) Put(b *png.EncoderBuffer) { p.b = b }

// Hub receives every rendered frame and serves the newest picture to any
// number of browsers.
type Hub struct {
	lastPoll atomic.Int64 // unix nanos of the newest viewer request

	// Touched only by Frame (one goroutine).
	prev []byte
	big  *image.RGBA
	buf  bytes.Buffer

	mu      sync.Mutex
	png     []byte
	ver     uint64
	changed chan struct{} // closed, then replaced, on every new picture
}

// NewHub returns an idle Hub, ready to be wired into a Runtime and served.
func NewHub() *Hub {
	return &Hub{changed: make(chan struct{})}
}

func (h *Hub) watched() bool {
	return time.Since(time.Unix(0, h.lastPoll.Load())) < viewerTTL
}

// Frame publishes img when somebody is watching and the picture differs from
// the last one. Must not retain img past the call — same convention as
// capture.Recorder.Frame. The host renders at a fixed rate, mostly the same
// picture, so the pixel compare is what keeps idle screens cheap.
func (h *Hub) Frame(img *image.NRGBA) {
	if !h.watched() {
		return
	}
	w, ht := img.Rect.Dx(), img.Rect.Dy()
	pix := img.Pix[:ht*img.Stride]
	if bytes.Equal(pix, h.prev) {
		return
	}
	h.prev = append(h.prev[:0], pix...)

	if h.big == nil || h.big.Rect.Dx() != w*scale || h.big.Rect.Dy() != ht*scale {
		h.big = image.NewRGBA(image.Rect(0, 0, w*scale, ht*scale))
	}
	upscale(h.big, img, scale)
	h.buf.Reset()
	if err := pngEnc.Encode(&h.buf, h.big); err != nil {
		return
	}
	pic := append([]byte(nil), h.buf.Bytes()...)

	h.mu.Lock()
	h.png = pic
	h.ver++
	close(h.changed)
	h.changed = make(chan struct{})
	h.mu.Unlock()
}

// upscale copies src into dst, every pixel becoming k x k pixels. The panel
// is opaque, so NRGBA and RGBA bytes are the same.
func upscale(dst *image.RGBA, src *image.NRGBA, k int) {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	for y := 0; y < h; y++ {
		in := src.Pix[y*src.Stride : y*src.Stride+w*4]
		first := dst.Pix[y*k*dst.Stride : y*k*dst.Stride+w*k*4]
		for x := 0; x < w; x++ {
			for j := 0; j < k; j++ {
				copy(first[(x*k+j)*4:], in[x*4:x*4+4])
			}
		}
		for j := 1; j < k; j++ {
			copy(dst.Pix[(y*k+j)*dst.Stride:], first)
		}
	}
}

// ServeHTTP serves the viewer page, or with ?frame=1 one PNG of the newest
// picture. With since equal to the newest version, a frame request waits up
// to waitMax for a new picture and answers 204 if none comes. The version is
// in the X-Mirror-Seq header.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	h.lastPoll.Store(time.Now().UnixNano())
	if r.URL.Query().Get("frame") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(viewerHTML)) //nolint:errcheck
		return
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)

	timeout := time.NewTimer(waitMax)
	defer timeout.Stop()
	for {
		h.mu.Lock()
		pic, ver, changed := h.png, h.ver, h.changed
		h.mu.Unlock()
		if pic != nil && ver != since {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("X-Mirror-Seq", strconv.FormatUint(ver, 10))
			w.Write(pic) //nolint:errcheck
			return
		}
		select {
		case <-changed:
		case <-timeout.C:
			h.lastPoll.Store(time.Now().UnixNano())
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNoContent)
			return
		case <-r.Context().Done():
			return
		}
	}
}

const viewerHTML = `<!doctype html><meta charset=utf-8><title>Push mirror</title>
<style>html,body{margin:0;background:#111;height:100%}canvas{width:960px;max-width:100%;display:block}</style>
<canvas id=c width=1920 height=320></canvas>
<script>
const ctx=document.getElementById('c').getContext('2d');let since=0;
const wait=ms=>new Promise(r=>setTimeout(r,ms));
(async function loop(){for(;;){try{
 const r=await fetch(location.pathname+'?frame=1&since='+since,{cache:'no-store'});
 if(r.status===200){since=r.headers.get('X-Mirror-Seq');ctx.drawImage(await createImageBitmap(await r.blob()),0,0);}
 else if(r.status!==204)await wait(500);
}catch(e){await wait(500);}}})();
</script>`
