// Package thru forwards Push's controls straight out as MIDI.
//
// Press a pad, another app receives a note. Turn an encoder, it receives a CC.
// It is the smallest module that actually sends MIDI, which makes it the thing
// that proves the whole output path — module -> host -> internal/midiout -> a
// port other software can see.
//
// It is also the identity case of a remapper: once mappings are configurable,
// "forward everything unchanged" is just the default table. So this is a
// building block rather than a throwaway probe.
//
// What it deliberately does NOT do:
//
//   - **No MPE.** Pad note-ons can arrive on channel 1 or rotate across
//     channels 2-16 depending on device state, and per-note pressure/slide/bend
//     follow the note's member channel. All of that is collapsed onto one output
//     channel here, and Expression events are ignored. Predictable output beats
//     faithful output for something whose job is to be verifiable.
//   - **No configuration.** The output channel is fixed until per-module
//     persistence lands.
package thru

import (
	"fmt"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

const (
	// outChannel is the MIDI channel everything is sent on, 1-16.
	outChannel = 1

	// padColour is the palette index a held pad lights up with. Lighting the
	// pad locally as well as sending the note means a silent output path is
	// obvious: the pad lights but nothing arrives.
	//
	// 11, matching core/push3/colors.go's NamedColors["green"] (#34C216).
	// That file itself was wrong until 2026-08-18 — it claimed "green" = 22
	// under an assumption (inherited from Push 2's colors.pyc) that only
	// even velocities carry a real colour. A live SysEx query of Push 3's own
	// palette (see docs/hardware-reference.md for the palette doc link) shows
	// every one of the 128 raw velocities is a distinct, real colour with no
	// gaps — colors.go has
	// since been corrected to match. This constant follows that correction.
	padColour = 11 // green

	// encoderCCBase maps encoders 1-8 onto CC 1-8.
	//
	// Deliberately NOT Push's own CC 71-78. Forwarding an encoder's source CC
	// number would imply this is a wire-level passthrough, and it is not — the
	// value is converted from relative to absolute, so the number should not
	// pretend to be the same control.
	encoderCCBase = 1

	// encoderStart is where each encoder's absolute value begins, so a first
	// turn in either direction is audible.
	encoderStart = 64

	logLines = 6
)

// Module forwards controls to a MIDI output port.
//
// Plain fields, no mutex: Handle and Draw never run concurrently.
type Module struct {
	host module.Host

	// held tracks sounding notes so they can be turned off on shutdown. Without
	// this, switching modules or quitting mid-press leaves a note ringing in
	// whatever is listening — the most annoying possible bug in a MIDI tool.
	held map[byte]bool

	encoders [8]int

	log      []string
	lastSent string
	sent     int
	errs     int
	lastErr  string

	start  time.Time
	frames int
}

// New returns the thru module.
func New() *Module {
	return &Module{held: map[byte]bool{}}
}

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:      "thru",
		Name:    "MIDI Thru (Test)",
		Author:  "Federico Pepe",
		Version: "1.0.0",

		// The host refuses to activate this module when no output port is
		// available, rather than letting every send fail quietly.
		NeedsMIDIOut: true,
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	m.start = time.Now()
	m.frames = 0
	for i := range m.encoders {
		m.encoders[i] = encoderStart
	}
	h.Log("forwarding to MIDI channel %d", outChannel)
	return nil
}

// Close silences anything still sounding. The host clears LEDs itself, but it
// knows nothing about notes in flight — only this module does.
func (m *Module) Close() error {
	for note := range m.held {
		if err := m.host.NoteOff(outChannel, note); err != nil {
			return fmt.Errorf("thru: releasing note %d: %w", note, err)
		}
		delete(m.held, note)
	}
	return nil
}

func (m *Module) push(line string) {
	m.log = append(m.log, line)
	if len(m.log) > logLines {
		m.log = m.log[len(m.log)-logLines:]
	}
}

// record folds a send result into the counters, so a failing output path shows
// up on screen instead of vanishing.
func (m *Module) record(desc string, err error) {
	if err != nil {
		m.errs++
		m.lastErr = err.Error()
		m.push("ERR  " + desc)
		return
	}
	m.sent++
	m.lastSent = desc
	m.push(desc)
}

func (m *Module) Handle(ev module.Event) {
	switch e := ev.(type) {
	case module.Pad:
		m.handlePad(e)

	case module.Encoder:
		m.handleEncoder(e)

	case module.Button:
		// Buttons forward their own CC number unchanged: it is already an
		// absolute 0/127 switch, so there is nothing to convert and nothing
		// misleading about keeping the number.
		val := byte(0)
		if e.Pressed {
			val = 127
		}
		err := m.host.SendCC(outChannel, e.CC, val)
		m.record(fmt.Sprintf("cc   %d = %d  (%s)", e.CC, val, buttonLabel(e)), err)

	case module.Touch, module.Expression:
		// Touch is not a control value, and MPE is out of scope — see the
		// package doc.
	}
}

func (m *Module) handlePad(e module.Pad) {
	if e.Pressed {
		// Note number is the pad's own note (36-99), so the grid maps onto a
		// receiving instrument the same way it does on Push.
		err := m.host.SendNote(outChannel, e.Note, e.Velocity)
		if err == nil {
			m.held[e.Note] = true
		}
		m.host.SetPad(e.Note, padColour)
		m.record(fmt.Sprintf("note %d on  vel %d", e.Note, e.Velocity), err)
		return
	}

	err := m.host.NoteOff(outChannel, e.Note)
	delete(m.held, e.Note)
	m.host.SetPad(e.Note, 0)
	m.record(fmt.Sprintf("note %d off", e.Note), err)
}

func (m *Module) handleEncoder(e module.Encoder) {
	// Only the eight screen encoders forward. Index -1 is volume/tempo/jog,
	// which have no obvious CC assignment and would surprise more than help.
	if e.Index < 0 || e.Index >= len(m.encoders) {
		return
	}
	// Relative to absolute: accumulate the signed delta and clamp. Encoders
	// accelerate, so the delta can be up to +/-11 — never treat it as one click.
	m.encoders[e.Index] = push3.ClampInt(m.encoders[e.Index]+e.Delta, 0, 127)

	cc := byte(encoderCCBase + e.Index)
	val := byte(m.encoders[e.Index])
	err := m.host.SendCC(outChannel, cc, val)
	m.record(fmt.Sprintf("cc   %d = %d  (enc %d)", cc, val, e.Index+1), err)
}

func buttonLabel(e module.Button) string {
	if e.Name != "" {
		return e.Name
	}
	return fmt.Sprintf("CC %d unmapped", e.CC)
}

func (m *Module) Draw(f *module.Frame) {
	m.frames++
	w, h := f.Size()
	t := m.host.Theme()

	f.Rect(0, 0, w, h, t.Black)

	// Title bar. Channel is shown because it is the one thing a listener has to
	// match, and it is not configurable yet.
	f.Rect(0, 0, w, 20, t.CrumbBg)
	f.Text(8, 14, fmt.Sprintf("pushapp - thru  ->  ch %d", outChannel), t.CrumbCol)
	el := time.Since(m.start).Seconds()
	if el < 0.001 {
		el = 0.001
	}
	stats := fmt.Sprintf("%d sent   %.0f fps", m.sent, float64(m.frames)/el)
	f.Text(w-8-text.Width(stats), 14, stats, t.Gray)

	// Left: encoder values as meters, since these are absolute now rather than
	// the accumulators the monitor shows.
	f.Text(10, 34, "ENCODERS -> CC 1-8", t.Gray)
	for i, v := range m.encoders {
		y := 44 + i*12
		f.Text(10, y+8, fmt.Sprintf("%d", i+1), t.Gray)
		f.Meter(24, y, 90, 8, float64(v)/127, t.Accent, t.DarkGray)
		f.Text(120, y+8, fmt.Sprintf("%3d", v), t.White)
	}

	// Middle: notes currently sounding. The interesting failure is a stuck note,
	// so it gets its own count rather than being buried in the log.
	mx := 170
	f.Text(mx, 34, "SOUNDING", t.Gray)
	f.Text(mx, 50, fmt.Sprintf("%d note(s)", len(m.held)), t.White)
	row := 0
	for note := range m.held {
		if row >= 6 {
			f.Text(mx, 66+row*14, "...", t.Gray)
			break
		}
		f.Text(mx, 66+row*14, fmt.Sprintf("%d", note), t.OnColor)
		row++
	}

	// Right: what was actually sent.
	lx := mx + 110
	f.Text(lx, 34, "SENT", t.Gray)
	for i, line := range m.log {
		c := t.White
		if i < len(m.log)-1 {
			c = t.Gray
		}
		f.Text(lx, 50+i*15, text.Truncate(line, 44), c)
	}

	// Bottom strip: errors get priority over the last success. A silent output
	// path is the failure this module exists to make visible.
	if m.errs > 0 {
		f.Rect(0, h-18, w, 18, t.OffColor)
		f.Text(8, h-5, fmt.Sprintf("%d send error(s): %s", m.errs,
			text.Truncate(m.lastErr, 90)), t.White)
		return
	}
	last := m.lastSent
	if last == "" {
		last = "press a pad or turn an encoder"
	}
	f.Rect(0, h-18, w, 18, t.StatusBg)
	f.Text(8, h-5, "last sent: "+last, t.StatusCol)
}
