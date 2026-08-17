package monitor

import (
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/module/moduletest"
)

// newTest wires the module to a fake host — no Push, no USB, no MIDI ports.
// This is the pattern every module should be testable with.
func newTest(t *testing.T) (*Module, *moduletest.Host) {
	t.Helper()
	h := &moduletest.Host{}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return m, h
}

// TestPadPressLightsAndReleasesLED is the behaviour the vertical slice proved on
// hardware, now checked without any.
func TestPadPressLightsAndReleasesLED(t *testing.T) {
	m, h := newTest(t)

	const note = 36 // bottom-left
	m.Handle(module.Pad{Note: note, Col: 0, Row: 0, Channel: 1, Velocity: 100, Pressed: true})

	if got := h.LitPads()[note]; got != padColour {
		t.Errorf("pad %d colour = %d, want %d", note, got, padColour)
	}
	if len(m.padsLit) != 1 {
		t.Errorf("padsLit has %d entries, want 1", len(m.padsLit))
	}

	m.Handle(module.Pad{Note: note, Pressed: false})
	if _, still := h.LitPads()[note]; still {
		t.Error("pad LED still lit after release")
	}
	if len(m.padsLit) != 0 {
		t.Errorf("padsLit has %d entries after release, want 0", len(m.padsLit))
	}
}

// TestEncoderAccumulatesSignedDelta guards the acceleration rule: encoders send
// deltas up to +/-11 on a fast turn, so the module must add the signed value
// rather than counting messages.
func TestEncoderAccumulatesSignedDelta(t *testing.T) {
	m, _ := newTest(t)

	m.Handle(module.Encoder{CC: push3.CCEncoder1, Index: 0, Delta: 7, Name: "Encoder 1"})
	m.Handle(module.Encoder{CC: push3.CCEncoder1, Index: 0, Delta: -3, Name: "Encoder 1"})
	if m.encoders[0] != 4 {
		t.Errorf("encoder 0 = %d, want 4 (7 then -3)", m.encoders[0])
	}

	// Index -1 is the volume/tempo/jog encoders: logged, but not accumulated
	// into the eight screen slots.
	before := m.encoders
	m.Handle(module.Encoder{CC: push3.CCJogWheel, Index: -1, Delta: 5, Name: "Jog Wheel"})
	if m.encoders != before {
		t.Error("an Index -1 encoder wrote into the screen-encoder accumulators")
	}
}

// TestLogIsBounded keeps a long session from growing the log without limit.
func TestLogIsBounded(t *testing.T) {
	m, _ := newTest(t)
	for i := 0; i < logLines*5; i++ {
		m.Handle(module.Button{CC: 20, Name: "Screen Bot 1", Pressed: true})
	}
	if len(m.log) != logLines {
		t.Errorf("log has %d lines, want %d", len(m.log), logLines)
	}
}

// TestUnmappedButtonIsStillReadable — a CC with no name must not log a blank.
func TestUnmappedButtonIsStillReadable(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Button{CC: 3, Name: "", Pressed: true})
	if len(m.log) != 1 {
		t.Fatalf("log has %d lines, want 1", len(m.log))
	}
	if m.log[0] == "btn  " {
		t.Error("unmapped CC logged with an empty name")
	}
}

// TestExpressionIsCountedNotLogged — MPE data is high-rate; logging every one
// would flood the screen and hide everything else.
func TestExpressionIsCountedNotLogged(t *testing.T) {
	m, _ := newTest(t)
	for i := 0; i < 50; i++ {
		m.Handle(module.Expression{Channel: 2, Kind: "pressure", Value: i})
	}
	if len(m.log) != 0 {
		t.Errorf("expression events wrote %d log lines, want 0", len(m.log))
	}
	if m.evCount != 50 {
		t.Errorf("evCount = %d, want 50", m.evCount)
	}
}

// TestDrawEmitsOnlySupportedOps is the guard against a module drawing into the
// void: every op it emits must be one the host can render.
func TestDrawEmitsOnlySupportedOps(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Pad{Note: 99, Col: 7, Row: 7, Channel: 1, Velocity: 127, Pressed: true})
	m.Handle(module.Touch{Note: 0, Name: "Encoder 1 Touch", Touched: true})

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

// TestDrawIsIdempotent — Draw must not mutate state that changes what it draws,
// or the display would depend on frame count rather than on input.
func TestDrawIsIdempotent(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Pad{Note: 60, Col: 0, Row: 3, Channel: 1, Velocity: 64, Pressed: true})

	f1 := module.NewFrame(960, 160)
	m.Draw(f1)
	n1 := len(f1.Ops())

	f2 := module.NewFrame(960, 160)
	m.Draw(f2)
	n2 := len(f2.Ops())

	if n1 != n2 {
		t.Errorf("Draw emitted %d ops then %d — output depends on call count", n1, n2)
	}
}

// TestReinitKeepsHistory — re-activating the monitor mid-session should not
// discard what it has already seen, which is the whole point of a monitor.
func TestReinitKeepsHistory(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Pad{Note: 40, Channel: 1, Velocity: 10, Pressed: true})
	if m.padCount != 1 {
		t.Fatalf("padCount = %d, want 1", m.padCount)
	}

	if err := m.Init(h); err != nil {
		t.Fatalf("re-Init: %v", err)
	}
	if m.padCount != 1 {
		t.Errorf("padCount = %d after re-Init, want 1 (history should survive)", m.padCount)
	}
	if m.frames != 0 {
		t.Errorf("frames = %d after re-Init, want 0 (fps is measured per activation)", m.frames)
	}
}
