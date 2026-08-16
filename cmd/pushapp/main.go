// Command pushapp is the vertical slice: one Go binary that owns Push's screen,
// reads its control surface, and lights the pads you press.
//
// It exists to prove the *stack*, not the protocol. The protocol was already
// confirmed (docs/feasibility.md §8), but across three languages — display in
// Go, MIDI in and LED out in Swift probes. This closes that gap: display over
// libusb, MIDI over an OS API, LEDs back out, one process, one language.
//
// Runs in co-existence mode: claims only the display interface, leaving Push's
// audio and MIDI with the OS. If Live holds the display, it degrades to a
// MIDI-only session rather than failing.
//
//	go run ./cmd/pushapp
//	go run ./cmd/pushapp -fps 60 -no-display
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/capture"
	"github.com/federico-pepe/push-tethered-app/internal/display"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
)

// padColour is the palette index pads light up with when pressed.
// 124 = white in core/push3/colors.go.
const padColour = 124

// state is everything the renderer draws, guarded because MIDI callbacks
// arrive on the driver's thread while the render loop reads on ours.
type state struct {
	mu sync.Mutex

	padsLit  map[byte]bool
	log      []string
	encoders [8]int
	padCount int
	evCount  int
	lastPad  string
}

func newState() *state {
	return &state{padsLit: map[byte]bool{}}
}

func (s *state) push(line string) {
	s.log = append(s.log, line)
	if len(s.log) > 6 {
		s.log = s.log[len(s.log)-6:]
	}
}

func (s *state) handle(ev pmidi.Event, port *pmidi.Port, ledsOn bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evCount++

	switch e := ev.(type) {
	case pmidi.Pad:
		if e.Pressed {
			s.padsLit[e.Note] = true
			s.padCount++
			s.lastPad = fmt.Sprintf("note %d  col %d row %d  ch%d  vel %d",
				e.Note, e.Col+1, e.Row+1, e.Channel, e.Velocity)
			s.push(fmt.Sprintf("pad  %d (%d,%d) ch%d vel %d", e.Note, e.Col+1, e.Row+1, e.Channel, e.Velocity))
			if ledsOn {
				_ = port.SetPad(e.Note, padColour)
			}
		} else {
			delete(s.padsLit, e.Note)
			if ledsOn {
				_ = port.SetPad(e.Note, 0)
			}
		}

	case pmidi.Button:
		n := e.Name
		if n == "" {
			n = fmt.Sprintf("CC %d (unmapped)", e.CC)
		}
		if e.Pressed {
			s.push("btn  " + n)
		}

	case pmidi.Encoder:
		if e.Index >= 0 && e.Index < 8 {
			s.encoders[e.Index] += e.Delta
		}
		s.push(fmt.Sprintf("enc  %s %+d", e.Name(), e.Delta))

	case pmidi.Touch:
		if e.Touched {
			s.push("touch " + e.Name)
		}

	case pmidi.Expression:
		// Per-note MPE data is high-rate; count it but do not log every one.
	}
}

func main() {
	fps := flag.Int("fps", 30, "display refresh rate")
	noDisplay := flag.Bool("no-display", false, "skip the display, run MIDI only")
	noLEDs := flag.Bool("no-leds", false, "do not drive pad LEDs")
	capturePath := flag.String("capture", "", "record the screen to a file (.mp4, .mov or .gif)")
	captureRaw := flag.Bool("capture-raw", false, "record the source image instead of panel-accurate BGR565 colour")
	flag.Parse()

	log.SetFlags(0)

	// ── MIDI ────────────────────────────────────────────────────────────────
	port, err := pmidi.Open()
	if err != nil {
		log.Fatalf("MIDI: %v", err)
	}
	defer port.Close()
	port.Clear()
	log.Printf("MIDI: connected to %q", port.Name())

	st := newState()
	if err := port.Listen(func(ev pmidi.Event) { st.handle(ev, port, !*noLEDs) }); err != nil {
		log.Fatalf("MIDI listen: %v", err)
	}

	// ── Display ─────────────────────────────────────────────────────────────
	var dev *display.Device
	if !*noDisplay {
		dev, err = display.Open()
		switch {
		case errors.Is(err, display.ErrBusy):
			// The documented degrade path: Live owns the screen, we keep MIDI.
			log.Printf("display: %v", err)
			log.Printf("display: continuing MIDI-only — quit Live to get the screen")
		case errors.Is(err, display.ErrNotFound):
			log.Fatalf("display: %v", err)
		case err != nil:
			log.Fatalf("display: %v", err)
		default:
			defer dev.Close()
			log.Printf("display: claimed %s", dev.Model())
		}
	}

	// ── Capture ─────────────────────────────────────────────────────────────
	// Recording taps the render output, so it costs no extra USB traffic and
	// cannot disturb what the panel shows.
	var rec capture.Recorder
	if *capturePath != "" {
		rec, err = capture.New(capture.Options{Path: *capturePath, FPS: *fps, Raw: *captureRaw})
		if err != nil {
			log.Fatalf("capture: %v", err)
		}
		mode := "panel-accurate"
		if *captureRaw {
			mode = "raw source"
		}
		log.Printf("capture: recording %s (%s)", rec.Path(), mode)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("running at %d fps — press pads, turn encoders. Ctrl-C to stop.", *fps)

	ticker := time.NewTicker(time.Second / time.Duration(*fps))
	defer ticker.Stop()

	start := time.Now()
	frames := 0
	for {
		select {
		case <-ctx.Done():
			shutdown(dev, port, rec, start, frames, st)
			return
		case <-ticker.C:
			// Render even with no display claimed, so -no-display and the
			// Live-owns-the-screen path can still be recorded.
			img := render(st, start, frames)

			if rec != nil {
				if err := rec.Frame(img); err != nil {
					log.Printf("capture: %v — recording stopped", err)
					rec = nil
				}
			}

			if dev == nil {
				frames++
				continue
			}
			if err := dev.WriteFrame(ctx, img); err != nil {
				if ctx.Err() != nil {
					shutdown(dev, port, rec, start, frames, st)
					return
				}
				log.Printf("frame %d: %v", frames, err)
				continue
			}
			frames++
		}
	}
}

func shutdown(dev *display.Device, port *pmidi.Port, rec capture.Recorder, start time.Time, frames int, st *state) {
	fmt.Println()
	log.Printf("stopping…")
	port.Clear()
	if dev != nil {
		_ = dev.Blank(context.Background())
	}
	if rec != nil {
		if err := rec.Close(); err != nil {
			log.Printf("capture: %v", err)
		} else {
			log.Printf("capture: wrote %s", rec.Path())
		}
	}
	el := time.Since(start)
	st.mu.Lock()
	defer st.mu.Unlock()
	log.Printf("%d frames in %s (%.1f fps), %d MIDI events, %d pad presses",
		frames, el.Round(time.Millisecond), float64(frames)/el.Seconds(), st.evCount, st.padCount)
}

// render draws the UI with the shared core/ widget toolkit — the same code
// that draws on a standalone Push 3.
func render(st *state, start time.Time, frames int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, push3.VisW, push3.VisH))
	t := widgets.Default

	st.mu.Lock()
	defer st.mu.Unlock()

	gfx.FillRect(img, 0, 0, push3.VisW, push3.VisH, t.Black)

	// Title bar.
	gfx.FillRect(img, 0, 0, push3.VisW, 20, t.CrumbBg)
	text.Draw(img, 8, 14, "pushapp - co-existence mode", t.CrumbCol)
	el := time.Since(start).Seconds()
	stats := fmt.Sprintf("%d events   %.0f fps", st.evCount, float64(frames)/max(el, 0.001))
	text.Draw(img, push3.VisW-8-text.Width(stats), 14, stats, t.Gray)

	// Left: live 8x8 grid mirror of which pads are held.
	const cell, gx, gy = 12, 10, 28
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			note := push3.PadNote(col, row)
			// Row 0 is the bottom row, so draw it last (lowest on screen).
			y := gy + (7-row)*cell
			c := t.DarkGray
			if st.padsLit[note] {
				c = t.OnColor
			}
			gfx.FillRect(img, gx+col*cell, y, cell-2, cell-2, c)
		}
	}
	text.Draw(img, gx, gy+8*cell+12, fmt.Sprintf("%d held", len(st.padsLit)), t.Gray)

	// Middle: encoder accumulators.
	ex := gx + 8*cell + 24
	text.Draw(img, ex, 34, "ENCODERS", t.Gray)
	for i, v := range st.encoders {
		col, row := i%2, i/2
		text.Draw(img, ex+col*90, 50+row*16, fmt.Sprintf("%d:%+d", i+1, v), t.White)
	}

	// Right: event log.
	lx := ex + 200
	text.Draw(img, lx, 34, "EVENTS", t.Gray)
	for i, line := range st.log {
		c := t.White
		if i < len(st.log)-1 {
			c = color.NRGBA{150, 150, 150, 255}
		}
		text.Draw(img, lx, 50+i*15, text.Truncate(line, 40), c)
	}

	// Bottom strip: last pad, proving the note->coordinate decode live.
	last := st.lastPad
	if last == "" {
		last = "press a pad"
	}
	gfx.FillRect(img, 0, push3.VisH-18, push3.VisW, 18, t.StatusBg)
	text.Draw(img, 8, push3.VisH-5, "last pad: "+last, t.StatusCol)

	return img
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
