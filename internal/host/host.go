package host

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/capture"
	"github.com/federico-pepe/push-tethered-app/internal/display"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
	"github.com/federico-pepe/push-tethered-app/internal/midiout"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
)

// eventBuf is how many input events may queue for the module goroutine.
//
// Generous on purpose. Push emits Active Sensing ~37x/second (filtered before it
// reaches here) and MPE expression data can be dense, but a module that keeps up
// will never see this fill. If it does fill, the module is wedged and the
// interesting fact is the drop count, not the events.
const eventBuf = 1024

// Options configures a Runtime.
type Options struct {
	FPS       int
	NoDisplay bool
	NoLEDs    bool
	Recorder  capture.Recorder

	// OpenMIDIOut obtains the output port, called at most once and only when a
	// module that declares NeedsMIDIOut is activated.
	//
	// A function rather than an already-open port, because on macOS and Linux
	// opening one *publishes it to the whole system*. Deciding from the set of
	// compiled-in modules was wrong: adding one module that sends MIDI would
	// make every run publish a port, even a run of a module that never sends.
	// Nil means no output is available.
	OpenMIDIOut func() (*midiout.Out, error)

	Theme      module.Theme
	ThemeIsSet bool
}

// Runtime owns the hardware and runs one module at a time.
//
// Threading model, which is the whole point of this type:
//
//   - MIDI arrives on the driver's thread. That thread does nothing but decode,
//     translate and enqueue. It never calls into a module and never blocks.
//   - One goroutine — the module goroutine — drains the queue and drives the
//     frame ticker. Handle and Draw therefore never overlap, and module authors
//     need no locks.
//   - LED and MIDI-out writes happen from module code, so from that same
//     goroutine. The mutex around the port exists only for the host's own
//     writes on shutdown and switching.
type Runtime struct {
	opts    Options
	port    *pmidi.Port
	dev     *display.Device
	modules []module.Module // compiled-in, fixed at construction — never mutated

	// installed holds process-loaded modules (see internal/host/procmod),
	// added by Install and removed by Uninstall. Separate from modules rather
	// than merged into one mutable slice: modules is a fixed set no lock is
	// ever needed for, and keeping it that way means every method that only
	// cares about compiled-in modules (there are none yet, but a future one
	// might) needs no lock either.
	installedMu sync.RWMutex
	installed   []*installedModule

	// portMu guards writes to the MIDI port. internal/midi.Port has no internal
	// synchronisation, and the host writes to it (Clear) from a different
	// goroutine than the module does.
	portMu sync.Mutex

	activeMu sync.RWMutex
	active   module.Module

	events  chan module.Event
	dropped atomic.Int64

	// out is the lazily-opened MIDI output port, plus a one-shot guard so a
	// failed open is not retried on every activation.
	outMu    sync.Mutex
	out      *midiout.Out
	outTried bool
	outErr   error

	frame *module.Frame
	img   *image.NRGBA

	frames  int
	evCount atomic.Int64
	start   time.Time
}

// New builds a Runtime around an already-open MIDI port and optional display.
// The caller owns opening and closing those, because the failure modes (Live
// holding the display, no Push connected) are the caller's to report.
func New(port *pmidi.Port, dev *display.Device, opts Options, modules ...module.Module) (*Runtime, error) {
	if len(modules) == 0 {
		return nil, errors.New("host: no modules registered")
	}
	if opts.FPS <= 0 {
		opts.FPS = 30
	}
	if !opts.ThemeIsSet {
		opts.Theme = widgets.Default
	}
	return &Runtime{
		opts:    opts,
		port:    port,
		dev:     dev,
		modules: modules,
		events:  make(chan module.Event, eventBuf),
		frame:   module.NewFrame(push3.VisW, push3.VisH),
		img:     image.NewNRGBA(image.Rect(0, 0, push3.VisW, push3.VisH)),
	}, nil
}

// ── Control API ────────────────────────────────────────────────────────────
//
// This is what the app UI binds to. Kept a plain Go interface rather than a
// socket protocol: the UI runs in-process (Wails), and a headless run needs none
// of it.

// List returns metadata for every available module — compiled-in first, then
// installed, each in the order they were added.
func (r *Runtime) List() []module.Meta {
	r.installedMu.RLock()
	defer r.installedMu.RUnlock()
	metas := make([]module.Meta, 0, len(r.modules)+len(r.installed))
	for _, m := range r.modules {
		metas = append(metas, m.Meta())
	}
	for _, im := range r.installed {
		metas = append(metas, im.mod.Meta())
	}
	return metas
}

// Active returns the running module's metadata, or the zero value if none.
func (r *Runtime) Active() module.Meta {
	r.activeMu.RLock()
	defer r.activeMu.RUnlock()
	if r.active == nil {
		return module.Meta{}
	}
	return r.active.Meta()
}

// Activate switches to the module with the given ID, closing the current one.
//
// Refuses a module that needs MIDI out when none is available: better to fail
// here, once and loudly, than to let every SendCC in a sequencer fail quietly.
func (r *Runtime) Activate(id string) error {
	next := r.findModule(id)
	if next == nil {
		return fmt.Errorf("no module with id %q", id)
	}
	// Open the output port only now, and only because this module asked for it.
	if next.Meta().NeedsMIDIOut {
		out, err := r.ensureMIDIOut()
		if err != nil {
			return fmt.Errorf("module %q needs a MIDI output port: %w "+
				"(on Windows, create one with loopMIDI and pass -midi-out)", id, err)
		}
		log.Printf("MIDI out: %q (%s)", out.Name(), out.Mode())
	}

	r.activeMu.Lock()
	prev := r.active
	r.active = nil
	r.activeMu.Unlock()

	if prev != nil {
		if err := prev.Close(); err != nil {
			log.Printf("module %s: close: %v", prev.Meta().ID, err)
		}
	}
	// Clear between modules so the previous one's LEDs never bleed into the
	// next. Cheap, and it makes a switch unambiguous.
	r.clearLEDs()
	r.drainEvents()

	if err := next.Init(&moduleHost{rt: r, id: next.Meta().ID}); err != nil {
		return fmt.Errorf("module %q: init: %w", id, err)
	}

	r.activeMu.Lock()
	r.active = next
	r.activeMu.Unlock()
	log.Printf("module: %s (%s) active", next.Meta().Name, next.Meta().ID)

	// A module only ever sees Store.Get/Set, never a path — but until a config
	// UI exists, a user editing settings by hand needs to know where the file
	// is. Purely informational: printed whether or not the file exists yet.
	if p, err := configFilePath(id); err == nil {
		log.Printf("module %s: config at %s", id, p)
	}
	return nil
}

// ── Run loop ───────────────────────────────────────────────────────────────

// Run starts listening for input and drives the display until ctx is done.
//
// Both Handle and Draw are called from this goroutine, which is what makes the
// no-locking guarantee in the module contract true.
func (r *Runtime) Run(ctx context.Context) error {
	if r.Active().ID == "" {
		if err := r.Activate(r.modules[0].Meta().ID); err != nil {
			return err
		}
	}

	// The driver thread's only job: translate and enqueue.
	if err := r.port.Listen(func(ev pmidi.Event) {
		r.evCount.Add(1)
		me := translate(ev)
		if me == nil {
			return
		}
		select {
		case r.events <- me:
		default:
			// Drop rather than block. Blocking here would stall the MIDI driver
			// thread, which is far worse than losing an event: the whole point
			// of the queue is that a slow module cannot reach back and wedge
			// the hardware. Dropping the newest keeps the queue in order.
			r.dropped.Add(1)
		}
	}); err != nil {
		return fmt.Errorf("MIDI listen: %w", err)
	}

	ticker := time.NewTicker(time.Second / time.Duration(r.opts.FPS))
	defer ticker.Stop()

	r.start = time.Now()
	log.Printf("running at %d fps — Ctrl-C to stop.", r.opts.FPS)

	for {
		select {
		case <-ctx.Done():
			return nil

		case ev := <-r.events:
			r.activeMu.RLock()
			m := r.active
			r.activeMu.RUnlock()
			if m != nil {
				m.Handle(ev)
			}

		case <-ticker.C:
			if err := r.drawFrame(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				if errors.Is(err, display.ErrDisconnected) {
					return err
				}
				log.Printf("frame %d: %v", r.frames, err)
				continue
			}
			r.frames++
		}
	}
}

func (r *Runtime) drawFrame(ctx context.Context) error {
	r.activeMu.RLock()
	m := r.active
	r.activeMu.RUnlock()
	if m == nil {
		return nil
	}

	r.frame.Reset()
	m.Draw(r.frame)

	// Clear to the theme background rather than allocating a fresh image each
	// frame: at 30fps a 960x160 NRGBA is 2.4MB/s of garbage for no reason.
	clear(r.img.Pix)
	stats := Render(r.frame, r.img, r.opts.Theme)
	if stats.Unknown > 0 || stats.Failed > 0 {
		// Once per frame at most, and only when something is actually wrong.
		log.Printf("module %s: %d unknown / %d failed ops",
			m.Meta().ID, stats.Unknown, stats.Failed)
	}

	if r.opts.Recorder != nil {
		if err := r.opts.Recorder.Frame(r.img); err != nil {
			log.Printf("capture: %v — recording stopped", err)
			r.opts.Recorder = nil
		}
	}

	if r.dev == nil {
		return nil
	}
	return r.dev.WriteFrame(ctx, r.img)
}

// drainEvents empties the queue, so a module does not inherit input aimed at its
// predecessor.
func (r *Runtime) drainEvents() {
	for {
		select {
		case <-r.events:
		default:
			return
		}
	}
}

// ensureMIDIOut opens the output port on first use and caches the result,
// success or failure.
//
// Caching the failure matters: without it, a module that sends on every pad
// press would retry a doomed open — and on Windows that means enumerating every
// MIDI port on the machine — dozens of times a second.
func (r *Runtime) ensureMIDIOut() (*midiout.Out, error) {
	r.outMu.Lock()
	defer r.outMu.Unlock()

	if r.outTried {
		return r.out, r.outErr
	}
	r.outTried = true

	if r.opts.OpenMIDIOut == nil {
		r.outErr = errors.New("no MIDI output is configured")
		return nil, r.outErr
	}
	out, err := r.opts.OpenMIDIOut()
	if err != nil {
		r.outErr = err
		return nil, err
	}
	r.out = out
	return out, nil
}

func (r *Runtime) clearLEDs() {
	r.portMu.Lock()
	defer r.portMu.Unlock()
	r.port.Clear()
}

// Shutdown closes the active module, clears the LEDs, blanks the screen and
// reports what happened. Always call it, including on SIGINT: a device left lit
// makes the next run ambiguous.
func (r *Runtime) Shutdown() {
	r.activeMu.Lock()
	m := r.active
	r.active = nil
	r.activeMu.Unlock()

	if m != nil {
		if err := m.Close(); err != nil {
			log.Printf("module %s: close: %v", m.Meta().ID, err)
		}
	}
	r.clearLEDs()
	if r.dev != nil {
		_ = r.dev.Blank(context.Background())
	}
	// The Runtime opened the port, so the Runtime closes it — on macOS and Linux
	// leaving it open would leave a phantom port advertised to the system.
	r.outMu.Lock()
	if r.out != nil {
		r.out.Close()
		r.out = nil
	}
	r.outMu.Unlock()
	if r.opts.Recorder != nil {
		if err := r.opts.Recorder.Close(); err != nil {
			log.Printf("capture: %v", err)
		} else {
			log.Printf("capture: wrote %s", r.opts.Recorder.Path())
		}
	}

	el := time.Since(r.start)
	log.Printf("%d frames in %s (%.1f fps), %d MIDI events",
		r.frames, el.Round(time.Millisecond), float64(r.frames)/el.Seconds(),
		r.evCount.Load())
	if d := r.dropped.Load(); d > 0 {
		log.Printf("WARNING: dropped %d input events — a module was too slow to "+
			"keep up with the queue", d)
	}
}

// ── module.Host implementation ─────────────────────────────────────────────

// moduleHost is the per-module view of the runtime. One per activation, so Log
// can tag lines with the module's ID and so a stale reference held by a closed
// module is easy to reason about.
type moduleHost struct {
	rt *Runtime
	id string
}

func (h *moduleHost) Device() pushmap.Device { return h.rt.port.Device() }
func (h *moduleHost) Theme() module.Theme    { return h.rt.opts.Theme }
func (h *moduleHost) SupportedOps() []string { return SupportedOps() }

func (h *moduleHost) Log(format string, args ...any) {
	log.Printf("%s: %s", h.id, fmt.Sprintf(format, args...))
}

func (h *moduleHost) SetPad(note, colour byte) {
	if h.rt.opts.NoLEDs {
		return
	}
	h.rt.portMu.Lock()
	defer h.rt.portMu.Unlock()
	_ = h.rt.port.SetPad(note, colour)
}

func (h *moduleHost) SetButton(cc, brightness byte) {
	if h.rt.opts.NoLEDs {
		return
	}
	h.rt.portMu.Lock()
	defer h.rt.portMu.Unlock()
	_ = h.rt.port.SetButton(cc, brightness)
}

func (h *moduleHost) out() (*midiout.Out, error) { return h.rt.ensureMIDIOut() }

func (h *moduleHost) SendCC(ch, cc, val byte) error {
	o, err := h.out()
	if err != nil {
		return err
	}
	return o.SendCC(ch, cc, val)
}

func (h *moduleHost) SendNote(ch, note, vel byte) error {
	o, err := h.out()
	if err != nil {
		return err
	}
	return o.SendNote(ch, note, vel)
}

func (h *moduleHost) NoteOff(ch, note byte) error {
	o, err := h.out()
	if err != nil {
		return err
	}
	return o.NoteOff(ch, note)
}

// Store gives the module its own JSON document, one file per module ID under
// the OS config directory. See store.go.
func (h *moduleHost) Store() module.Store { return newStore(h.id) }

// ── Event translation ──────────────────────────────────────────────────────

// translate converts a decoder event into an ABI event.
//
// Names are resolved here rather than in the module: encoder naming needs the
// per-device tables, and a module should not have to know that CC 15 is Push 2's
// Swing encoder and Push 3's Tempo press.
func translate(ev pmidi.Event) module.Event {
	switch e := ev.(type) {
	case pmidi.Pad:
		return module.Pad{
			Note: e.Note, Col: e.Col, Row: e.Row,
			Channel: e.Channel, Velocity: e.Velocity, Pressed: e.Pressed,
		}
	case pmidi.Button:
		return module.Button{CC: e.CC, Name: e.Name, Pressed: e.Pressed}
	case pmidi.Encoder:
		return module.Encoder{CC: e.CC, Index: e.Index, Delta: e.Delta, Name: e.Name()}
	case pmidi.Touch:
		return module.Touch{Note: e.Note, Name: e.Name, Touched: e.Touched}
	case pmidi.Expression:
		return module.Expression{Channel: e.Channel, Kind: e.Kind, Value: e.Value}
	}
	return nil
}
