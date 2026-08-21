// Package monitor is the control-surface monitor: a live mirror of what Push is
// sending.
//
// It is the port of what cmd/pushapp drew before the host existed, and it stays
// useful for the same reason it was written — it is the fastest way to see
// whether input decoding, LED output and the display are all working at once.
// Every later module gets debugged by comparing against this one.
//
// It is also the reference for what a module looks like: no USB, no MIDI ports,
// no goroutines, no locks. State is plain fields, because Handle and Draw are
// guaranteed never to run concurrently.
package monitor

import (
	"fmt"
	"image/color"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// padColour is the palette index a held pad lights up with.
// 120 = white in core/push3/colors.go. (Was 124 before that file's
// 2026-08-18 correction — a live SysEx query of Push 3's own palette showed
// every one of the 128 raw velocities is a distinct real colour, which moved
// every named entry; see colors.go's own header comment for the full story.)
const padColour = 120

// logLines is how many events the on-screen log keeps.
const logLines = 6

// Module mirrors the control surface. Fields are unguarded on purpose — see the
// package doc.
type Module struct {
	host module.Host

	padsLit  map[byte]bool
	log      []string
	encoders [8]int
	padCount int
	evCount  int
	lastPad  string

	start  time.Time
	frames int
}

// New returns the monitor module.
func New() *Module {
	return &Module{padsLit: map[byte]bool{}}
}

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:          "monitor",
		Name:        "Push MIDI Monitor",
		Author:      "Federico Pepe",
		Version:     "1.0.0",
		Description: "Shows every pad, button, and encoder event as it arrives",
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	m.start = time.Now()
	m.frames = 0
	// Deliberately not resetting counters or the log: re-activating the monitor
	// mid-session should not throw away what it has seen.
	h.Log("watching %s", h.Device())
	return nil
}

func (m *Module) Close() error { return nil }

func (m *Module) push(line string) {
	m.log = append(m.log, line)
	if len(m.log) > logLines {
		m.log = m.log[len(m.log)-logLines:]
	}
}

func (m *Module) Handle(ev module.Event) {
	m.evCount++

	switch e := ev.(type) {
	case module.Pad:
		if e.Pressed {
			m.padsLit[e.Note] = true
			m.padCount++
			// Col/Row are 0-indexed from the bottom-left; displayed 1-indexed
			// because that is how a person counts pads.
			m.lastPad = fmt.Sprintf("note %d  col %d row %d  ch%d  vel %d",
				e.Note, e.Col+1, e.Row+1, e.Channel, e.Velocity)
			m.push(fmt.Sprintf("pad  %d (%d,%d) ch%d vel %d",
				e.Note, e.Col+1, e.Row+1, e.Channel, e.Velocity))
			m.host.SetPad(e.Note, padColour)
		} else {
			delete(m.padsLit, e.Note)
			m.host.SetPad(e.Note, 0)
		}

	case module.Button:
		n := e.Name
		if n == "" {
			n = fmt.Sprintf("CC %d (unmapped)", e.CC)
		}
		if e.Pressed {
			m.push("btn  " + n)
		}

	case module.Encoder:
		if e.Index >= 0 && e.Index < len(m.encoders) {
			// Accumulate the signed delta — encoders accelerate, so one message
			// is not one click.
			m.encoders[e.Index] += e.Delta
		}
		m.push(fmt.Sprintf("enc  %s %+d", e.Name, e.Delta))

	case module.Touch:
		if e.Touched {
			m.push("touch " + e.Name)
		}

	case module.Expression:
		// Per-note MPE data is high-rate: counted above, not logged.
	}
}

func (m *Module) Draw(f *module.Frame) {
	m.frames++
	w, h := f.Size()
	t := m.host.Theme()

	f.Rect(0, 0, w, h, t.Black)

	// Title bar, with a live frame rate. The module counts its own Draw calls
	// rather than asking the host, since that is the same number and keeps the
	// contract smaller.
	f.Header(0, w, 20, "pushapp - monitor")
	el := time.Since(m.start).Seconds()
	if el < 0.001 {
		el = 0.001
	}
	stats := fmt.Sprintf("%d events   %.0f fps", m.evCount, float64(m.frames)/el)
	f.Text(w-8-text.Width(stats), 14, stats, t.Gray)

	// Left: 8x8 mirror of which pads are held. DrawPadGrid's row 0 = bottom
	// convention is the same flip that used to be inline here — this is
	// still the note-to-coordinate decode proof, just drawn by the shared
	// widget instead of a hand-rolled loop.
	const cell, gx, gy = 12, 10, 28
	colors := make([][]color.NRGBA, 8)
	for row := 0; row < 8; row++ {
		colors[row] = make([]color.NRGBA, 8)
		for col := 0; col < 8; col++ {
			note := push3.PadNote(col, row)
			c := t.DarkGray
			if m.padsLit[note] {
				c = t.OnColor
			}
			colors[row][col] = c
		}
	}
	f.PadGrid(gx, gy, cell, colors)
	f.Text(gx, gy+8*cell+12, fmt.Sprintf("%d held", len(m.padsLit)), t.Gray)

	// Middle: encoder accumulators.
	ex := gx + 8*cell + 24
	f.Text(ex, 34, "ENCODERS", t.Gray)
	for i, v := range m.encoders {
		col, row := i%2, i/2
		f.Text(ex+col*90, 50+row*16, fmt.Sprintf("%d:%+d", i+1, v), t.White)
	}

	// Right: event log, most recent brightest.
	lx := ex + 200
	f.Text(lx, 34, "EVENTS", t.Gray)
	for i, line := range m.log {
		c := t.White
		if i < len(m.log)-1 {
			c = color.NRGBA{150, 150, 150, 255}
		}
		f.Text(lx, 50+i*15, text.Truncate(line, 40), c)
	}

	// Bottom strip: the last pad in full, proving note->coordinate decode live.
	last := m.lastPad
	if last == "" {
		last = "press a pad"
	}
	f.StatusBar(h-18, w, 18, "last pad: "+last, false)
}
