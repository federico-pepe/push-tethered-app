// Package seq is an 8-step, 8-lane gate sequencer.
//
// The pad grid IS the sequencer: columns are steps, rows are pitch lanes.
// Press a pad to toggle that lane's step on or off; the playhead advances on
// its own and lights the current column, sending a note for every active lane
// in it. It exists to prove the parts of the module contract that monitor and
// thru do not exercise: MIDI driven by wall-clock timing rather than input,
// and real persistence — the pattern and tempo survive a restart.
//
// Deliberately minimal, matching the size of a phase-2 proof rather than a
// finished instrument:
//
//   - 8 steps, not 16. The pad grid is 8 columns wide; doubling up columns for
//     16 steps is a real feature, not a proof, and is left for later.
//   - One lane = one fixed chromatic note (see baseNote). No scale, no
//     per-lane pitch editing.
//   - A step's gate lasts until the next step. No per-step gate length or
//     velocity editing.
//   - Timing is advanced inside Draw, using time.Now(). The advance logic
//     itself (tick) takes an explicit time so it is testable without a real
//     clock — see seq_test.go.
//
// Optionally follows an external MIDI clock instead of its own wall-clock
// timing (see NeedsMIDIIn, handleExternalMIDI): while clock bytes are
// actively arriving, they — not tick() — advance the step, so the sequencer
// locks to whatever is sending them. No external clock connected is the
// common case and needs no configuration; wall-clock timing is the default
// and the fallback the moment external ticks stop arriving.
package seq

import (
	"fmt"
	"image/color"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

const (
	lanes = 8
	steps = 8

	// outChannel is the MIDI channel every note goes out on.
	outChannel = 1

	// baseNote is lane 0's pitch; lane i is baseNote+i. Plain chromatic rather
	// than a scale, so the mapping is obvious and easy to verify by ear.
	baseNote = 60 // middle C

	// stepsPerBeat makes each step an eighth note at the current BPM — a
	// natural feel for an 8-step pattern (one 4/4 bar) without adding a
	// separate "resolution" setting to edit.
	stepsPerBeat = 2

	minBPM, maxBPM, defaultBPM = 40, 240, 120

	// clockTicksPerQuarter is the MIDI clock standard: 24 ticks per quarter
	// note, regardless of tempo. ticksPerStep converts that to this
	// sequencer's own resolution (stepsPerBeat steps per quarter note).
	clockTicksPerQuarter = 24
	ticksPerStep         = clockTicksPerQuarter / stepsPerBeat

	// externalClockTimeout is how long without a clock byte before falling
	// back to wall-clock timing. Long enough that a single dropped tick at
	// any realistic tempo (even 40 BPM, 24 ticks/quarter, is ~62ms/tick)
	// never trips it; short enough that unplugging the clock source is
	// noticed well within one bar.
	externalClockTimeout = 2 * time.Second

	// Both match core/push3/colors.go's NamedColors ("green" and "white").
	// That file itself was corrected 2026-08-18: it used to claim "green" = 22
	// and "white" = 124 (a Push-2-derived assumption that only even velocities
	// carry a real colour), found wrong when a pad rendered pink instead of
	// green on real hardware. A live SysEx query of Push 3's own palette
	// (see docs/hardware-reference.md for the palette doc link) shows every
	// one of the 128 raw velocities is a distinct, real colour with no gaps;
	// colors.go now reflects that.
	// These are pad LEDs (SetPad below) — note that 122, which these constants
	// used to sit near, is white *only for CC buttons*; for a pad Note On,
	// velocity 122 is a near-black grey. The button-brightness alias and the
	// pad-palette index share the same byte value by hardware coincidence, not
	// by rule — always resolve a colour by name, not by picking a nearby int.
	activeColour  = 11  // green ("green" in NamedColors, #34C216)
	playingColour = 120 // white for a PAD ("white" in NamedColors, #FFFFFF)

	logLines = 5
)

// pattern is what gets persisted: the grid and the tempo. Steps is
// [lane][step] rather than [step][lane] so a lane's row reads contiguously,
// matching how a single pad row is scanned.
type pattern struct {
	BPM   int                `json:"bpm"`
	Steps [lanes][steps]bool `json:"steps"`
}

// Module is the sequencer. Plain fields: Handle and Draw never run
// concurrently, and the wall-clock tick happens inside Draw, not on a
// separate goroutine, so it needs no locking either.
type Module struct {
	host module.Host

	pattern pattern
	playing bool

	// playStart anchors step 0; step index is derived from elapsed time, not
	// accumulated per-tick, so a slow frame cannot cause steps to be skipped.
	playStart time.Time
	curStep   int
	haveStep  bool // false until the first tick after Init/play

	// External MIDI clock state — see handleExternalMIDI. extTicks counts
	// clock bytes (0-ticksPerStep-1) within the current step; lastExtClock is
	// when the last one arrived, which is what isExternallySynced checks
	// against externalClockTimeout. wasExternalSynced lets tick() notice the
	// exact frame sync is lost, so it can re-anchor playStart instead of
	// jumping using a now-stale one.
	extTicks          int
	lastExtClock      time.Time
	wasExternalSynced bool

	log     []string
	sent    int
	lastErr string

	frames int
	start  time.Time
}

// New returns the sequencer module.
func New() *Module { return &Module{} }

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:           "seq",
		Name:         "Step Sequencer (Test)",
		Author:       "Federico Pepe",
		Version:      "1.0.0",
		Description:  "8-step pad-grid sequencer, wall-clock or external-MIDI-clock driven",
		NeedsMIDIOut: true,
		NeedsMIDIIn:  true,
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	m.start = time.Now()
	m.frames = 0

	// Defaults are set before Get, per the Store contract: nothing stored yet
	// leaves them as they are, so a first run gets a sensible pattern rather
	// than a silent empty one.
	doc := pattern{BPM: defaultBPM}
	if err := h.Store().Get(&doc); err != nil {
		h.Log("loading pattern: %v (starting from defaults)", err)
		doc = pattern{BPM: defaultBPM}
	}
	if doc.BPM < minBPM || doc.BPM > maxBPM {
		doc.BPM = defaultBPM
	}
	m.pattern = doc

	m.playing = true
	m.haveStep = false
	m.playStart = time.Now()

	h.Log("BPM %d, %d steps — pads toggle, top Play button starts/stops", m.pattern.BPM, steps)
	m.lightGrid()
	return nil
}

// Close silences whatever is currently sounding. The host clears LEDs but
// knows nothing about notes in flight.
func (m *Module) Close() error {
	if !m.haveStep {
		return nil
	}
	return m.releaseStep(m.curStep)
}

func (m *Module) push(line string) {
	m.log = append(m.log, line)
	if len(m.log) > logLines {
		m.log = m.log[len(m.log)-logLines:]
	}
}

func (m *Module) fail(err error) {
	m.lastErr = err.Error()
	m.push("ERR  " + m.lastErr)
}

func (m *Module) save() {
	if err := m.host.Store().Set(m.pattern); err != nil {
		m.host.Log("saving pattern: %v", err)
	}
}

func (m *Module) Handle(ev module.Event) {
	switch e := ev.(type) {
	case module.Pad:
		if !e.Pressed {
			return
		}
		lane, step := e.Row, e.Col
		if lane < 0 || lane >= lanes || step < 0 || step >= steps {
			return
		}
		m.pattern.Steps[lane][step] = !m.pattern.Steps[lane][step]
		m.host.SetPad(e.Note, m.padColour(lane, step))
		m.save()

	case module.Button:
		if e.Pressed && e.CC == push3.CCPlay {
			m.togglePlay()
		}

	case module.Encoder:
		// Only the first screen encoder controls tempo.
		//
		// Known rough edge: tick() derives the step index from total elapsed
		// time / step duration rather than incrementing a counter, so changing
		// BPM mid-playback can visibly jump the current step, not just its
		// speed. A real transport would re-anchor playStart to keep the
		// current step's phase; not worth the complexity for an 8-step proof.
		if e.Index != 0 {
			return
		}
		bpm := push3.ClampInt(m.pattern.BPM+e.Delta, minBPM, maxBPM)
		if bpm == m.pattern.BPM {
			return
		}
		m.pattern.BPM = bpm
		m.save()

	case module.ExternalMIDI:
		m.handleExternalMIDI(e)
	}
}

// handleExternalMIDI decodes the one byte this module cares about — clock,
// start, continue, stop — and ignores everything else (notes, CC, anything
// a sender chose to also put on the same port). Raw system realtime
// messages carry no channel and no data bytes, so a length/status check is
// all the decoding there is.
func (m *Module) handleExternalMIDI(e module.ExternalMIDI) {
	if len(e.Raw) == 0 {
		return
	}
	switch e.Raw[0] {
	case 0xF8: // Timing Clock
		m.onExternalClock()
	case 0xFA: // Start
		m.onExternalStart()
	case 0xFB: // Continue
		m.onExternalContinue()
	case 0xFC: // Stop
		m.onExternalStop()
	}
}

// onExternalClock advances the sequencer by one MIDI clock tick. Once
// ticksPerStep ticks have accumulated, it crosses a step boundary exactly
// like tick()'s wall-clock path does: release whatever the previous step was
// sounding, trigger the new one.
func (m *Module) onExternalClock() {
	m.lastExtClock = time.Now()
	if !m.playing {
		return
	}
	m.extTicks++
	if m.extTicks < ticksPerStep {
		return
	}
	m.extTicks = 0
	if m.haveStep {
		if err := m.releaseStep(m.curStep); err != nil {
			m.fail(err)
		}
	}
	m.curStep = (m.curStep + 1) % steps
	m.haveStep = true
	m.triggerStep(m.curStep)
}

// onExternalStart begins playback at step 0 immediately — MIDI Start means
// "begin now", not "begin after the first tick arrives", matching how a real
// transport reacts to it.
func (m *Module) onExternalStart() {
	m.lastExtClock = time.Now()
	if m.haveStep {
		if err := m.releaseStep(m.curStep); err != nil {
			m.fail(err)
		}
	}
	m.extTicks = 0
	m.curStep = 0
	m.haveStep = true
	if !m.playing {
		m.playing = true
		m.host.SetButton(push3.CCPlay, 127)
	}
	m.triggerStep(0)
}

// onExternalContinue resumes from wherever playback was left — unlike Start
// it does not reset the step or the tick count.
func (m *Module) onExternalContinue() {
	m.lastExtClock = time.Now()
	if !m.playing {
		m.playing = true
		m.host.SetButton(push3.CCPlay, 127)
	}
}

// onExternalStop mirrors togglePlay's stop path, driven by the clock link
// instead of the Play button.
func (m *Module) onExternalStop() {
	m.lastExtClock = time.Now()
	if !m.playing {
		return
	}
	if m.haveStep {
		if err := m.releaseStep(m.curStep); err != nil {
			m.fail(err)
		}
	}
	m.haveStep = false
	m.playing = false
	m.host.SetButton(push3.CCPlay, 0)
}

// isExternallySynced reports whether an external clock is actively driving
// the sequencer right now — ticks arriving recently enough that tick()'s
// wall-clock path should stand down rather than also advancing the step.
func (m *Module) isExternallySynced() bool {
	return !m.lastExtClock.IsZero() && time.Since(m.lastExtClock) < externalClockTimeout
}

func (m *Module) togglePlay() {
	m.playing = !m.playing
	if m.playing {
		m.playStart = time.Now()
		m.haveStep = false
		m.host.SetButton(push3.CCPlay, 127)
	} else {
		if m.haveStep {
			if err := m.releaseStep(m.curStep); err != nil {
				m.fail(err)
			}
		}
		m.haveStep = false
		m.host.SetButton(push3.CCPlay, 0)
	}
}

// tick advances the sequencer to whatever step `now` falls in, firing note-off
// for the previous step and note-on for the new one on a boundary crossing.
//
// Takes an explicit time rather than reading the clock itself so the advance
// logic is testable without a real clock or a sleep.
func (m *Module) tick(now time.Time) {
	if !m.playing {
		return
	}
	if m.isExternallySynced() {
		// An external clock owns stepping right now — see onExternalClock.
		m.wasExternalSynced = true
		return
	}
	if m.wasExternalSynced {
		// Just lost the external clock (timeout, or it was never that
		// active to begin with): re-anchor from now rather than resuming
		// wall-clock timing from a playStart that may be arbitrarily stale.
		m.playStart = now
		m.haveStep = false
		m.wasExternalSynced = false
	}
	stepDur := time.Duration(float64(time.Minute) / float64(m.pattern.BPM) / stepsPerBeat)
	if stepDur <= 0 {
		return
	}
	idx := int(now.Sub(m.playStart)/stepDur) % steps

	if m.haveStep && idx == m.curStep {
		return
	}
	if m.haveStep {
		if err := m.releaseStep(m.curStep); err != nil {
			m.fail(err)
		}
	}
	m.curStep = idx
	m.haveStep = true
	m.triggerStep(idx)
}

func (m *Module) triggerStep(step int) {
	any := false
	for lane := 0; lane < lanes; lane++ {
		if !m.pattern.Steps[lane][step] {
			continue
		}
		any = true
		note := byte(baseNote + lane)
		if err := m.host.SendNote(outChannel, note, 100); err != nil {
			m.fail(err)
			continue
		}
		m.sent++
	}
	if any {
		m.push(fmt.Sprintf("step %d", step+1))
	}
	m.lightGrid()
}

func (m *Module) releaseStep(step int) error {
	var firstErr error
	for lane := 0; lane < lanes; lane++ {
		if !m.pattern.Steps[lane][step] {
			continue
		}
		note := byte(baseNote + lane)
		if err := m.host.NoteOff(outChannel, note); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// lightGrid redraws every pad LED from the current pattern plus playhead
// position. Called on toggle and on every step rather than incrementally,
// which is simpler and cheap: 64 MIDI bytes, not a hot loop.
func (m *Module) lightGrid() {
	for lane := 0; lane < lanes; lane++ {
		for step := 0; step < steps; step++ {
			note := push3.PadNote(step, lane)
			m.host.SetPad(note, m.padColour(lane, step))
		}
	}
}

func (m *Module) padColour(lane, step int) byte {
	active := m.pattern.Steps[lane][step]
	playhead := m.haveStep && step == m.curStep
	switch {
	case playhead && active:
		return playingColour
	case playhead:
		return activeColour // dim marker so the playhead is visible even on empty steps
	case active:
		return activeColour
	default:
		return 0
	}
}

func (m *Module) Draw(f *module.Frame) {
	m.frames++
	m.tick(time.Now())

	w, h := f.Size()
	t := m.host.Theme()
	f.Rect(0, 0, w, h, t.Black)

	state := "playing"
	if !m.playing {
		state = "stopped"
	}
	clockSrc := "internal"
	if m.isExternallySynced() {
		clockSrc = "EXT CLOCK"
	}
	f.Header(0, w, 20, fmt.Sprintf("pushapp - seq  BPM %d  [%s]  %s", m.pattern.BPM, state, clockSrc))

	// Grid mirror, laid out the same way monitor's is: row 0 is the bottom row
	// on the hardware, so DrawPadGrid draws it lowest on screen.
	const cell, gx, gy = 12, 10, 28
	colors := make([][]color.NRGBA, lanes)
	for lane := 0; lane < lanes; lane++ {
		colors[lane] = make([]color.NRGBA, steps)
		for step := 0; step < steps; step++ {
			c := t.DarkGray
			switch {
			case m.haveStep && step == m.curStep && m.pattern.Steps[lane][step]:
				c = t.OnColor
			case m.haveStep && step == m.curStep:
				c = t.Select
			case m.pattern.Steps[lane][step]:
				c = t.Accent
			}
			colors[lane][step] = c
		}
	}
	f.PadGrid(gx, gy, cell, colors)

	// Right: recent activity.
	lx := gx + steps*cell + 24
	f.Text(lx, 34, "EVENTS", t.Gray)
	for i, line := range m.log {
		c := t.White
		if i < len(m.log)-1 {
			c = t.Gray
		}
		f.Text(lx, 50+i*15, line, c)
	}

	// A send failure gets priority over the routine status — a silently broken
	// output path is the failure worth surfacing loudest.
	if m.lastErr != "" {
		f.Rect(0, h-18, w, 18, t.OffColor)
		f.Text(8, h-5, "send error: "+text.Truncate(m.lastErr, 90), t.White)
		return
	}
	last := "press a pad to toggle a step"
	if m.sent > 0 {
		last = fmt.Sprintf("%d notes sent", m.sent)
	}
	f.Rect(0, h-18, w, 18, t.StatusBg)
	f.Text(8, h-5, last, t.StatusCol)
}
