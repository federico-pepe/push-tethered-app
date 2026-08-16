// Package capture records what the app draws on Push's screen to a video file.
//
// The frames come from the render path, before the USB encode, so recording
// costs no extra USB traffic and cannot disturb the display.
//
// # Panel-accurate by default
//
// Push's panel is BGR565 — 16 bits per pixel — so the RGBA image we render is
// not what the hardware actually shows. By default this package round-trips
// each frame through core/display.ToBGR565/FromBGR565, so the recording has the
// same colour banding the panel does. Pass Raw to record the source image
// instead, which looks better but is not what you saw.
package capture

import (
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	coredisplay "github.com/federico-pepe/ableton-push-hack/core/display"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

// Recorder accepts rendered frames and writes them to a file.
type Recorder interface {
	// Frame records one frame. Must not retain img.
	Frame(img *image.NRGBA) error
	// Close finalises the file and reports where it landed.
	Close() error
	// Path is the output file.
	Path() string
}

// Options configures a recorder.
type Options struct {
	Path string // output file; extension selects the format
	FPS  int    // source frame rate
	Raw  bool   // record the source image rather than panel-accurate colour
}

// New returns a Recorder for the given path. `.mp4`/`.mov` use ffmpeg, which
// must be on PATH; `.gif` is encoded in-process with no external dependency.
func New(o Options) (Recorder, error) {
	if o.FPS <= 0 {
		o.FPS = 30
	}
	switch ext := strings.ToLower(filepath.Ext(o.Path)); ext {
	case ".mp4", ".mov":
		return newFFmpeg(o)
	case ".gif":
		return newGIF(o), nil
	case "":
		return nil, fmt.Errorf("capture path %q has no extension: use .mp4, .mov or .gif", o.Path)
	default:
		return nil, fmt.Errorf("unsupported capture format %q: use .mp4, .mov or .gif", ext)
	}
}

// panelize round-trips img through the panel's BGR565 encoding so the
// recording shows the colour depth the hardware actually renders.
func panelize(img *image.NRGBA) *image.NRGBA {
	return coredisplay.FromBGR565(coredisplay.ToBGR565(img)[:push3.FrameBytes])
}

// ── ffmpeg ──────────────────────────────────────────────────────────────────

type ffmpegRec struct {
	cmd  *exec.Cmd
	in   *os.File
	path string
	raw  bool
	n    int
	errb *strings.Builder
}

func newFFmpeg(o Options) (Recorder, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found on PATH (needed for %s; .gif needs no external tool): %w",
			filepath.Ext(o.Path), err)
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("creating pipe: %w", err)
	}

	// yuv420p for broad player compatibility; both dimensions are even so no
	// scaling is needed. -y overwrites, since re-running a capture is normal.
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", push3.VisW, push3.VisH),
		"-r", strconv.Itoa(o.FPS),
		"-i", "-",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
		o.Path,
	)
	cmd.Stdin = pr
	var errb strings.Builder
	cmd.Stderr = &errb

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}
	pr.Close() // the child owns the read end now

	return &ffmpegRec{cmd: cmd, in: pw, path: o.Path, raw: o.Raw, errb: &errb}, nil
}

func (f *ffmpegRec) Path() string { return f.path }

func (f *ffmpegRec) Frame(img *image.NRGBA) error {
	if !f.raw {
		img = panelize(img)
	}
	// image.NewNRGBA gives Stride == 4*width, so Pix is exactly the rawvideo
	// rgba layout ffmpeg expects and can be written without repacking.
	if _, err := f.in.Write(img.Pix); err != nil {
		return fmt.Errorf("writing frame %d to ffmpeg: %w", f.n, err)
	}
	f.n++
	return nil
}

func (f *ffmpegRec) Close() error {
	f.in.Close() // EOF tells ffmpeg to finalise the container
	if err := f.cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg failed after %d frames: %w: %s", f.n, err, strings.TrimSpace(f.errb.String()))
	}
	return nil
}

// ── GIF (pure Go, no external dependency) ───────────────────────────────────

type gifRec struct {
	g       *gif.GIF
	path    string
	raw     bool
	delay   int // hundredths of a second per stored frame
	stride  int // store every Nth frame
	n       int
	maxFrms int
}

func newGIF(o Options) Recorder {
	// GIF timing is in hundredths of a second, so 30fps is not representable.
	// Store every 3rd frame at ~10fps, which also keeps the file sane.
	stride := o.FPS / 10
	if stride < 1 {
		stride = 1
	}
	return &gifRec{
		g:       &gif.GIF{},
		path:    o.Path,
		raw:     o.Raw,
		delay:   10,
		stride:  stride,
		maxFrms: 600, // ~60s at 10fps; a guard, not a target
	}
}

func (g *gifRec) Path() string { return g.path }

func (g *gifRec) Frame(img *image.NRGBA) error {
	defer func() { g.n++ }()
	if g.n%g.stride != 0 || len(g.g.Image) >= g.maxFrms {
		return nil
	}
	if !g.raw {
		img = panelize(img)
	}
	p := image.NewPaletted(img.Bounds(), palette.Plan9)
	draw.FloydSteinberg.Draw(p, img.Bounds(), img, image.Point{})
	g.g.Image = append(g.g.Image, p)
	g.g.Delay = append(g.g.Delay, g.delay)
	return nil
}

func (g *gifRec) Close() error {
	if len(g.g.Image) == 0 {
		return fmt.Errorf("no frames captured")
	}
	f, err := os.Create(g.path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", g.path, err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, g.g); err != nil {
		return fmt.Errorf("encoding gif: %w", err)
	}
	return nil
}
