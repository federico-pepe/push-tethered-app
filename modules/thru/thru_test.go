package thru

import (
	"testing"

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

// TestDeclaresNeedsMIDIOut — without this the host would happily activate the
// module with no output port and every send would fail quietly.
func TestDeclaresNeedsMIDIOut(t *testing.T) {
	if !New().Meta().NeedsMIDIOut {
		t.Error("thru must declare NeedsMIDIOut")
	}
}

// TestPadSendsNoteOnAndOff covers the main path, including that the pad's own
// note number is what goes out.
func TestPadSendsNoteOnAndOff(t *testing.T) {
	m, h := newTest(t)

	m.Handle(module.Pad{Note: 60, Channel: 1, Velocity: 100, Pressed: true})
	if len(h.MIDI) != 1 {
		t.Fatalf("sent %d messages, want 1", len(h.MIDI))
	}
	got := h.MIDI[0]
	if got.Kind != "note" || got.Ch != outChannel || got.Num != 60 || got.Val != 100 {
		t.Errorf("note on = %+v, want note ch%d num60 val100", got, outChannel)
	}
	// And the pad lights, so a dead output path is visibly distinguishable from
	// a dead input path.
	if h.LitPads()[60] != padColour {
		t.Errorf("pad 60 colour = %d, want %d", h.LitPads()[60], padColour)
	}

	m.Handle(module.Pad{Note: 60, Pressed: false})
	if len(h.MIDI) != 2 {
		t.Fatalf("sent %d messages, want 2", len(h.MIDI))
	}
	if got := h.MIDI[1]; got.Kind != "noteoff" || got.Num != 60 {
		t.Errorf("note off = %+v, want noteoff num60", got)
	}
	if _, still := h.LitPads()[60]; still {
		t.Error("pad still lit after release")
	}
}

// TestMPEPadsCollapseToOneChannel — pad note-ons can arrive on channels 2-16
// depending on device state. Output must be predictable regardless.
func TestMPEPadsCollapseToOneChannel(t *testing.T) {
	m, h := newTest(t)
	for ch := 1; ch <= 16; ch++ {
		m.Handle(module.Pad{Note: 36, Channel: ch, Velocity: 64, Pressed: true})
	}
	for i, w := range h.MIDI {
		if w.Ch != outChannel {
			t.Errorf("message %d sent on channel %d, want %d", i, w.Ch, outChannel)
		}
	}
}

// TestCloseReleasesHeldNotes is the one that matters most in practice: quitting
// or switching modules mid-press must not leave a note ringing in the receiver.
func TestCloseReleasesHeldNotes(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Pad{Note: 36, Velocity: 100, Pressed: true})
	m.Handle(module.Pad{Note: 40, Velocity: 100, Pressed: true})
	m.Handle(module.Pad{Note: 44, Velocity: 100, Pressed: true})
	m.Handle(module.Pad{Note: 40, Pressed: false}) // released normally

	h.Reset()
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	offs := map[byte]bool{}
	for _, w := range h.MIDI {
		if w.Kind != "noteoff" {
			t.Errorf("Close sent a %q message, want only noteoff", w.Kind)
			continue
		}
		offs[w.Num] = true
	}
	if !offs[36] || !offs[44] {
		t.Errorf("Close released %v, want notes 36 and 44", offs)
	}
	if offs[40] {
		t.Error("Close re-released note 40, which was already released")
	}
	if len(m.held) != 0 {
		t.Errorf("%d notes still tracked as held after Close", len(m.held))
	}
}

// TestEncoderRelativeToAbsolute pins the conversion: encoders send signed
// deltas and accelerate, CC needs an absolute 0-127.
func TestEncoderRelativeToAbsolute(t *testing.T) {
	m, h := newTest(t)

	m.Handle(module.Encoder{Index: 0, Delta: 10})
	if got := h.MIDI[0]; got.Kind != "cc" || got.Num != encoderCCBase || got.Val != encoderStart+10 {
		t.Errorf("encoder 1 +10 = %+v, want cc %d val %d",
			got, encoderCCBase, encoderStart+10)
	}

	// Accumulates rather than resending the delta.
	m.Handle(module.Encoder{Index: 0, Delta: -4})
	if got := h.MIDI[1].Val; got != encoderStart+6 {
		t.Errorf("after -4, value = %d, want %d", got, encoderStart+6)
	}

	// Encoder 8 maps to CC 8, not to Push's source CC.
	m.Handle(module.Encoder{Index: 7, Delta: 1})
	if got := h.MIDI[2].Num; got != encoderCCBase+7 {
		t.Errorf("encoder 8 CC = %d, want %d", got, encoderCCBase+7)
	}
}

// TestEncoderClamps — a value outside 0-127 is not a valid CC and would be
// masked into something arbitrary on the wire.
func TestEncoderClamps(t *testing.T) {
	m, h := newTest(t)
	for i := 0; i < 40; i++ {
		m.Handle(module.Encoder{Index: 0, Delta: 11}) // fast turn, repeatedly
	}
	for i := 0; i < 80; i++ {
		m.Handle(module.Encoder{Index: 1, Delta: -11})
	}
	for _, w := range h.MIDI {
		if w.Val > 127 {
			t.Fatalf("sent CC value %d, above 127", w.Val)
		}
	}
	if m.encoders[0] != 127 {
		t.Errorf("encoder 1 = %d, want clamped to 127", m.encoders[0])
	}
	if m.encoders[1] != 0 {
		t.Errorf("encoder 2 = %d, want clamped to 0", m.encoders[1])
	}
}

// TestNonScreenEncodersAreIgnored — volume, tempo and jog arrive with Index -1
// and have no sensible CC assignment.
func TestNonScreenEncodersAreIgnored(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Encoder{CC: 70, Index: -1, Delta: 5, Name: "Jog Wheel"})
	if len(h.MIDI) != 0 {
		t.Errorf("an Index -1 encoder sent %d messages, want 0", len(h.MIDI))
	}
}

// TestButtonForwardsItsOwnCC — buttons are already absolute switches, so the
// number is kept.
func TestButtonForwardsItsOwnCC(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Button{CC: 20, Name: "Screen Bot 1", Pressed: true})
	m.Handle(module.Button{CC: 20, Name: "Screen Bot 1", Pressed: false})

	if len(h.MIDI) != 2 {
		t.Fatalf("sent %d messages, want 2", len(h.MIDI))
	}
	if h.MIDI[0].Num != 20 || h.MIDI[0].Val != 127 {
		t.Errorf("press = %+v, want cc 20 val 127", h.MIDI[0])
	}
	if h.MIDI[1].Val != 0 {
		t.Errorf("release = %+v, want val 0", h.MIDI[1])
	}
}

// TestTouchAndExpressionAreIgnored — out of scope, and forwarding high-rate MPE
// data would flood the port.
func TestTouchAndExpressionAreIgnored(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Touch{Note: 0, Name: "Encoder 1 Touch", Touched: true})
	for i := 0; i < 20; i++ {
		m.Handle(module.Expression{Channel: 2, Kind: "pressure", Value: i})
	}
	if len(h.MIDI) != 0 {
		t.Errorf("touch/expression sent %d messages, want 0", len(h.MIDI))
	}
}

// TestSendFailuresAreCountedNotSwallowed — a silent output path is the failure
// this module exists to expose, so it must be visible rather than logged away.
func TestSendFailuresAreCountedNotSwallowed(t *testing.T) {
	h := &moduletest.Host{NoMIDIOut: true}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}

	m.Handle(module.Pad{Note: 36, Velocity: 100, Pressed: true})
	m.Handle(module.Encoder{Index: 0, Delta: 1})

	if m.errs != 2 {
		t.Errorf("errs = %d, want 2", m.errs)
	}
	if m.sent != 0 {
		t.Errorf("sent = %d, want 0", m.sent)
	}
	if m.lastErr == "" {
		t.Error("lastErr is empty; the screen would show no reason")
	}
	// A note that failed to send must not be tracked as sounding, or Close would
	// try to release something that never started.
	if len(m.held) != 0 {
		t.Errorf("%d notes held after a failed send, want 0", len(m.held))
	}
}

// TestDrawEmitsOnlySupportedOps guards against drawing into the void.
func TestDrawEmitsOnlySupportedOps(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Pad{Note: 36, Velocity: 100, Pressed: true})
	m.Handle(module.Encoder{Index: 2, Delta: 3})

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

// TestDrawShowsErrorsOverLastSent — when the output path is broken, that is the
// only thing worth the bottom strip.
func TestDrawShowsErrorsOverLastSent(t *testing.T) {
	h := &moduletest.Host{NoMIDIOut: true}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatal(err)
	}
	m.Handle(module.Pad{Note: 36, Velocity: 1, Pressed: true})

	f := module.NewFrame(960, 160)
	m.Draw(f)
	if m.errs == 0 {
		t.Fatal("expected a recorded error")
	}
	// Cheap structural check: the error branch returns early, so no "last sent"
	// text is emitted.
	for _, op := range f.Ops() {
		if op.Kind == "text" && contains(string(op.Params), "last sent") {
			t.Error("drew \"last sent\" while errors were outstanding")
		}
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(hay); i++ {
			if hay[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// TestDrawTextIsASCII guards the class of bug "Draw emitted only known op
// kinds" cannot catch: a rendered string containing a non-ASCII character,
// which the host's sanitiser would silently turn into "?" rather than fail.
func TestDrawTextIsASCII(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Pad{Note: 36, Velocity: 100, Pressed: true})
	m.Handle(module.Encoder{Index: 2, Delta: 3})
	m.Handle(module.Button{CC: 20, Name: "Screen Bot 1", Pressed: true})

	f := module.NewFrame(960, 160)
	m.Draw(f)
	if bad := moduletest.NonASCIIStrings(f); len(bad) != 0 {
		t.Errorf("Draw emitted non-ASCII text: %q", bad)
	}
}
