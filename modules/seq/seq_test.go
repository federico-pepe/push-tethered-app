package seq

import (
	"testing"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/module/moduletest"
)

func newTest(t *testing.T) (*Module, *moduletest.Host) {
	t.Helper()
	h := &moduletest.Host{}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return m, h
}

func TestDeclaresNeedsMIDIOut(t *testing.T) {
	if !New().Meta().NeedsMIDIOut {
		t.Error("seq must declare NeedsMIDIOut")
	}
}

func TestDefaultsWhenNothingStored(t *testing.T) {
	m, _ := newTest(t)
	if m.pattern.BPM != defaultBPM {
		t.Errorf("BPM = %d, want default %d", m.pattern.BPM, defaultBPM)
	}
	if !m.playing {
		t.Error("a fresh sequencer should start playing")
	}
}

// TestPadTogglesStepAndLightsLED covers the core edit interaction: press toggles
// on, press again toggles off, and the pad LED always reflects the new state.
func TestPadTogglesStepAndLightsLED(t *testing.T) {
	m, h := newTest(t)
	h.Reset() // Init's lightGrid() already wrote 64 pad states

	note := push3.PadNote(3, 2) // step 3, lane 2
	m.Handle(module.Pad{Note: note, Col: 3, Row: 2, Pressed: true})
	if !m.pattern.Steps[2][3] {
		t.Fatal("step did not turn on")
	}
	if h.LitPads()[note] == 0 {
		t.Error("pad LED not lit after toggling a step on")
	}

	m.Handle(module.Pad{Note: note, Col: 3, Row: 2, Pressed: true})
	if m.pattern.Steps[2][3] {
		t.Error("second press did not toggle the step back off")
	}

	// Release must not toggle — only a press does.
	m.Handle(module.Pad{Note: note, Col: 3, Row: 2, Pressed: true})
	before := m.pattern.Steps[2][3]
	m.Handle(module.Pad{Note: note, Col: 3, Row: 2, Pressed: false})
	if m.pattern.Steps[2][3] != before {
		t.Error("a pad release toggled a step")
	}
}

// TestPatternPersistsAcrossReinit is the actual point of building a real
// store: a pattern edited in one session must be there in the next.
func TestPatternPersistsAcrossReinit(t *testing.T) {
	h := &moduletest.Host{}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatal(err)
	}
	m.Handle(module.Pad{Note: push3.PadNote(0, 0), Col: 0, Row: 0, Pressed: true})
	m.Handle(module.Pad{Note: push3.PadNote(5, 4), Col: 5, Row: 4, Pressed: true})
	m.Handle(module.Encoder{Index: 0, Delta: 20})
	wantBPM := defaultBPM + 20

	m2 := New()
	if err := m2.Init(h); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if !m2.pattern.Steps[0][0] || !m2.pattern.Steps[4][5] {
		t.Errorf("pattern did not survive re-init: %+v", m2.pattern.Steps)
	}
	if m2.pattern.BPM != wantBPM {
		t.Errorf("BPM = %d after re-init, want %d", m2.pattern.BPM, wantBPM)
	}
}

// TestEncoderClampsBPM guards against a runaway tempo, which would turn
// stepDur non-positive and could wedge tick's timing math.
func TestEncoderClampsBPM(t *testing.T) {
	m, _ := newTest(t)
	for i := 0; i < 50; i++ {
		m.Handle(module.Encoder{Index: 0, Delta: -11})
	}
	if m.pattern.BPM != minBPM {
		t.Errorf("BPM = %d, want clamped to %d", m.pattern.BPM, minBPM)
	}
	for i := 0; i < 50; i++ {
		m.Handle(module.Encoder{Index: 0, Delta: 11})
	}
	if m.pattern.BPM != maxBPM {
		t.Errorf("BPM = %d, want clamped to %d", m.pattern.BPM, maxBPM)
	}
}

func TestOtherEncodersDoNotAffectTempo(t *testing.T) {
	m, _ := newTest(t)
	before := m.pattern.BPM
	m.Handle(module.Encoder{Index: 1, Delta: 50})
	m.Handle(module.Encoder{Index: -1, Delta: 50})
	if m.pattern.BPM != before {
		t.Errorf("BPM changed to %d from a non-index-0 encoder", m.pattern.BPM)
	}
}

// TestTickFiresOnStepBoundary is the timing proof: notes must fire at the
// right wall-clock moments, using a synthetic clock so the test has no sleeps
// and no flakiness.
func TestTickFiresOnStepBoundary(t *testing.T) {
	m, h := newTest(t)
	m.pattern.BPM = 120 // step duration = 60/120/2 = 0.25s
	m.pattern.Steps[0][0] = true
	m.pattern.Steps[0][1] = true
	h.Reset()

	t0 := m.playStart
	m.tick(t0) // first tick, step 0
	if len(h.MIDI) != 1 || h.MIDI[0].Kind != "note" {
		t.Fatalf("first tick sent %+v, want one note-on", h.MIDI)
	}
	if got := h.MIDI[0].Num; got != baseNote {
		t.Errorf("step 0 lane 0 note = %d, want %d", got, baseNote)
	}

	m.tick(t0.Add(100 * time.Millisecond)) // still inside step 0
	if len(h.MIDI) != 1 {
		t.Errorf("tick mid-step sent %d messages, want 0 more", len(h.MIDI)-1)
	}

	m.tick(t0.Add(250 * time.Millisecond)) // crosses into step 1
	if len(h.MIDI) != 3 {
		t.Fatalf("at the step-1 boundary, got %d messages, want 3 (off, on)", len(h.MIDI))
	}
	if h.MIDI[1].Kind != "noteoff" || h.MIDI[1].Num != baseNote {
		t.Errorf("boundary message 1 = %+v, want noteoff %d", h.MIDI[1], baseNote)
	}
	if h.MIDI[2].Kind != "note" || h.MIDI[2].Num != baseNote {
		t.Errorf("boundary message 2 = %+v, want note-on %d (lane 0 also active in step 1)", h.MIDI[2], baseNote)
	}
}

// TestEmptyStepStillReleasesPrevious — a step with no active lanes must still
// turn off whatever the previous step was sounding.
func TestEmptyStepStillReleasesPrevious(t *testing.T) {
	m, h := newTest(t)
	m.pattern.BPM = 120
	m.pattern.Steps[3][0] = true // step 0 sounds, step 1 is empty
	h.Reset()

	t0 := m.playStart
	m.tick(t0)
	m.tick(t0.Add(250 * time.Millisecond))

	if len(h.MIDI) != 2 {
		t.Fatalf("got %d messages, want 2 (on then off)", len(h.MIDI))
	}
	if h.MIDI[1].Kind != "noteoff" {
		t.Errorf("second message = %+v, want noteoff", h.MIDI[1])
	}
}

// TestStoppedDoesNotAdvance — tick must be inert while paused.
func TestStoppedDoesNotAdvance(t *testing.T) {
	m, h := newTest(t)
	m.pattern.Steps[0][0] = true
	m.togglePlay() // now stopped
	h.Reset()

	m.tick(time.Now())
	m.tick(time.Now().Add(time.Second))
	if len(h.MIDI) != 0 {
		t.Errorf("tick sent %d messages while stopped, want 0", len(h.MIDI))
	}
}

// TestPlayButtonTogglesAndReleasesNotes covers the CCPlay button end to end,
// including that stopping mid-note releases it.
func TestPlayButtonTogglesAndReleasesNotes(t *testing.T) {
	m, h := newTest(t)
	m.pattern.Steps[0][0] = true
	h.Reset()

	m.tick(m.playStart) // step 0 sounding

	m.Handle(module.Button{CC: push3.CCPlay, Pressed: true})
	if m.playing {
		t.Fatal("Play button press did not stop playback")
	}
	found := false
	for _, w := range h.MIDI {
		if w.Kind == "noteoff" && w.Num == baseNote {
			found = true
		}
	}
	if !found {
		t.Error("stopping did not release the sounding note")
	}

	h.Reset()
	m.Handle(module.Button{CC: push3.CCPlay, Pressed: true})
	if !m.playing {
		t.Error("second Play press did not resume")
	}
}

// TestPlayButtonReleaseIsIgnored — only a press should toggle transport.
func TestPlayButtonReleaseIsIgnored(t *testing.T) {
	m, _ := newTest(t)
	before := m.playing
	m.Handle(module.Button{CC: push3.CCPlay, Pressed: false})
	if m.playing != before {
		t.Error("a Play button release toggled playback")
	}
}

func TestOtherButtonsAreIgnored(t *testing.T) {
	m, _ := newTest(t)
	before := m.playing
	m.Handle(module.Button{CC: 20, Pressed: true})
	if m.playing != before {
		t.Error("an unrelated button affected playback")
	}
}

// TestCloseReleasesSoundingNote — quitting mid-step must not leave a note
// ringing in whatever is listening.
func TestCloseReleasesSoundingNote(t *testing.T) {
	m, h := newTest(t)
	m.pattern.Steps[0][0] = true
	m.tick(m.playStart)
	h.Reset()

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(h.MIDI) != 1 || h.MIDI[0].Kind != "noteoff" || h.MIDI[0].Num != baseNote {
		t.Errorf("Close sent %+v, want one noteoff for %d", h.MIDI, baseNote)
	}
}

// TestCloseBeforeAnyTickDoesNotPanic — Close can run before Draw ever fires a
// single tick (e.g. activate then immediately switch away).
func TestCloseBeforeAnyTickDoesNotPanic(t *testing.T) {
	m, _ := newTest(t)
	if err := m.Close(); err != nil {
		t.Errorf("Close before any tick returned an error: %v", err)
	}
}

// TestSendFailureIsCountedNotSwallowed mirrors thru's guard: a broken output
// path must be visible.
func TestSendFailureIsCountedNotSwallowed(t *testing.T) {
	h := &moduletest.Host{NoMIDIOut: true}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatal(err)
	}
	m.pattern.Steps[0][0] = true

	m.tick(m.playStart)
	if m.lastErr == "" {
		t.Error("a failed send left lastErr empty")
	}
}

// TestDrawEmitsOnlySupportedOps guards against drawing into the void.
func TestDrawEmitsOnlySupportedOps(t *testing.T) {
	m, h := newTest(t)
	m.pattern.Steps[1][1] = true

	f := module.NewFrame(960, 160)
	m.Draw(f)

	if len(f.Ops()) == 0 {
		t.Fatal("Draw emitted no ops")
	}
	if f.Failed() != 0 {
		t.Errorf("Draw produced %d unmarshalable ops", f.Failed())
	}
	supported := map[string]bool{}
	for _, k := range h.SupportedOps() {
		supported[k] = true
	}
	for _, op := range f.Ops() {
		if !supported[op.Kind] {
			t.Errorf("Draw emitted unsupported op %q", op.Kind)
		}
	}
}

// TestDrawAdvancesTiming — Draw is where tick actually gets called in
// production, so it must not be a no-op wired in only for tests.
func TestDrawAdvancesTiming(t *testing.T) {
	m, h := newTest(t)
	m.pattern.BPM = 240 // fastest allowed: step = 60/240/2 = 0.125s
	m.pattern.Steps[0][0] = true
	h.Reset()

	m.Draw(module.NewFrame(960, 160))
	if len(h.MIDI) == 0 {
		t.Fatal("Draw did not advance the sequencer at all")
	}
}

// TestDrawTextIsASCII guards the class of bug "Draw emitted only known op
// kinds" cannot catch: a rendered string containing a non-ASCII character,
// which the host's sanitiser would silently turn into "?" rather than fail.
func TestDrawTextIsASCII(t *testing.T) {
	m, _ := newTest(t)
	m.pattern.Steps[1][1] = true
	m.tick(m.playStart)

	f := module.NewFrame(960, 160)
	m.Draw(f)
	if bad := moduletest.NonASCIIStrings(f); len(bad) != 0 {
		t.Errorf("Draw emitted non-ASCII text: %q", bad)
	}
}
