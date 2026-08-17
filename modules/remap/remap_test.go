package remap

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

func TestDeclaresNeedsMIDIOut(t *testing.T) {
	if !New().Meta().NeedsMIDIOut {
		t.Error("remap must declare NeedsMIDIOut")
	}
}

// TestNoOverridesBehavesLikeThru is the whole point of the package: with an
// empty config, a pad press, an encoder turn and a button press must produce
// exactly what thru would.
func TestNoOverridesBehavesLikeThru(t *testing.T) {
	m, h := newTest(t)
	if len(m.overrides) != 0 {
		t.Fatalf("fresh module has %d overrides, want 0", len(m.overrides))
	}

	m.Handle(module.Pad{Note: 60, Channel: 1, Velocity: 100, Pressed: true})
	if got := h.MIDI[0]; got.Kind != "note" || got.Ch != outChannel || got.Num != 60 || got.Val != 100 {
		t.Errorf("passthrough pad = %+v, want note ch%d num60 val100", got, outChannel)
	}

	m.Handle(module.Encoder{CC: 71, Index: 0, Delta: 10})
	if got := h.MIDI[1]; got.Kind != "cc" || got.Num != encoderCCBase || got.Val != encoderStart+10 {
		t.Errorf("passthrough encoder = %+v, want cc %d val %d", got, encoderCCBase, encoderStart+10)
	}

	m.Handle(module.Button{CC: 20, Name: "Screen Bot 1", Pressed: true})
	if got := h.MIDI[2]; got.Kind != "cc" || got.Num != 20 || got.Val != 127 {
		t.Errorf("passthrough button = %+v, want cc 20 val 127", got)
	}
}

// TestNoteOverrideRemapsAndScales is the case documented in the package doc:
// pad note 40 remapped to note 45 with velocity rescaled into 20-100.
func TestNoteOverrideRemapsAndScales(t *testing.T) {
	m, h := newTest(t)
	m.overrides["note:40"] = MidiMapping{OutType: "note", OutCh: 3, OutNum: 45, OutMin: 20, OutMax: 100}

	m.Handle(module.Pad{Note: 40, Channel: 1, Velocity: 127, Pressed: true})
	if len(h.MIDI) != 1 {
		t.Fatalf("sent %d messages, want 1", len(h.MIDI))
	}
	got := h.MIDI[0]
	if got.Kind != "note" || got.Ch != 3 || got.Num != 45 {
		t.Fatalf("mapped note = %+v, want note ch3 num45", got)
	}
	if got.Val != 100 {
		t.Errorf("velocity 127 scaled into [20,100] = %d, want 100 (top of range)", got.Val)
	}

	// A different, unmapped pad must still pass through unchanged.
	m.Handle(module.Pad{Note: 41, Channel: 1, Velocity: 50, Pressed: true})
	if got := h.MIDI[1]; got.Num != 41 || got.Val != 50 {
		t.Errorf("unmapped pad 41 = %+v, want passthrough note41 val50", got)
	}
}

// TestNoteOverrideReleaseSendsNoteOff — velocity 0 (or a pad release) must
// still resolve to the mapped output note, not the source note.
func TestNoteOverrideReleaseSendsNoteOff(t *testing.T) {
	m, h := newTest(t)
	m.overrides["note:40"] = MidiMapping{OutType: "note", OutCh: 1, OutNum: 45, OutMin: 20, OutMax: 100}

	m.Handle(module.Pad{Note: 40, Velocity: 100, Pressed: true})
	m.Handle(module.Pad{Note: 40, Pressed: false})

	if len(h.MIDI) != 2 {
		t.Fatalf("sent %d messages, want 2", len(h.MIDI))
	}
	off := h.MIDI[1]
	if off.Kind != "noteoff" || off.Num != 45 {
		t.Errorf("release = %+v, want noteoff on the mapped note 45", off)
	}
}

// TestCCOverrideOnButton covers an absolute (non-relative) CC rule.
func TestCCOverrideOnButton(t *testing.T) {
	m, h := newTest(t)
	m.overrides["cc:20"] = MidiMapping{OutType: "cc", OutCh: 2, OutNum: 7, OutMin: 0, OutMax: 127}

	m.Handle(module.Button{CC: 20, Pressed: true})
	if got := h.MIDI[0]; got.Kind != "cc" || got.Ch != 2 || got.Num != 7 || got.Val != 127 {
		t.Errorf("mapped button = %+v, want cc ch2 num7 val127", got)
	}
	m.Handle(module.Button{CC: 20, Pressed: false})
	if got := h.MIDI[1]; got.Val != 0 {
		t.Errorf("release = %+v, want val 0", got)
	}
}

// TestEncoderOverrideAccumulatesIntoRuleRange checks the relative-source path:
// the delta accumulates into the RULE's OutMin/OutMax, not 0-127, and the
// stored accumulator is the CLAMPED value — ported unchanged from
// push-manager's own remapAccum, which re-clamps on every delta rather than
// tracking an unclamped running total. So a first turn of +5 against
// OutMin 10 clamps straight to 10 (starting accumulator is 0, same as
// push-manager's), and the next delta is added to that clamped 10, not to the
// raw 5.
func TestEncoderOverrideAccumulatesIntoRuleRange(t *testing.T) {
	m, h := newTest(t)
	m.overrides["cc:71"] = MidiMapping{OutType: "cc", OutCh: 1, OutNum: 50, OutMin: 10, OutMax: 20}

	m.Handle(module.Encoder{CC: 71, Index: 0, Delta: 5})
	if got := h.MIDI[0]; got.Num != 50 || got.Val != 10 {
		t.Errorf("first turn = %+v, want cc50 val10 (0+5 clamped up to OutMin)", got)
	}

	m.Handle(module.Encoder{CC: 71, Index: 0, Delta: 3})
	if got := h.MIDI[1].Val; got != 13 {
		t.Errorf("after +3 more, val = %d, want 13 (10 + 3, accum stores the clamped value)", got)
	}

	// And clamps at the top of the range too.
	m.Handle(module.Encoder{CC: 71, Index: 0, Delta: 20})
	if got := h.MIDI[2].Val; got != 20 {
		t.Errorf("after a large turn, val = %d, want clamped to OutMax 20", got)
	}
}

// TestUnmappedEncoderIndexIsIgnored — volume/tempo/jog get no default mapping,
// same as thru, whether or not an override happens to exist for their CC.
func TestUnmappedEncoderIndexIsIgnored(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Encoder{CC: 70, Index: -1, Delta: 5, Name: "Jog Wheel"})
	if len(h.MIDI) != 0 {
		t.Errorf("an Index -1 encoder sent %d messages, want 0", len(h.MIDI))
	}
}

// TestOverridesPersistAcrossReinit exercises the real store, the phase-2
// point of this module existing at all.
func TestOverridesPersistAcrossReinit(t *testing.T) {
	h := &moduletest.Host{}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatal(err)
	}
	rule := MidiMapping{OutType: "note", OutCh: 1, OutNum: 45, OutMin: 0, OutMax: 127, Name: "test rule"}
	if err := h.Store().Set(doc{Overrides: map[string]MidiMapping{"note:40": rule}}); err != nil {
		t.Fatalf("seeding the store: %v", err)
	}

	m2 := New()
	if err := m2.Init(h); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	got, ok := m2.overrides["note:40"]
	if !ok {
		t.Fatal("override did not survive re-init")
	}
	if got != rule {
		t.Errorf("loaded rule = %+v, want %+v", got, rule)
	}
}

// TestSrcKeyIsChannelAgnostic is the documented, deliberate deviation from the
// ported push-manager model: a pad's MPE channel rotates between sessions, so
// a note override must match regardless of which channel a given press
// arrived on.
func TestSrcKeyIsChannelAgnostic(t *testing.T) {
	m, h := newTest(t)
	m.overrides["note:40"] = MidiMapping{OutType: "note", OutCh: 1, OutNum: 45, OutMin: 0, OutMax: 127}

	for ch := 1; ch <= 16; ch++ {
		h.Reset()
		m.Handle(module.Pad{Note: 40, Channel: ch, Velocity: 100, Pressed: true})
		if len(h.MIDI) != 1 || h.MIDI[0].Num != 45 {
			t.Errorf("channel %d: override did not match, got %+v", ch, h.MIDI)
		}
	}
}

// TestTouchAndExpressionAreIgnored mirrors thru: out of scope regardless of
// overrides, since there is no sensible override target for either.
func TestTouchAndExpressionAreIgnored(t *testing.T) {
	m, h := newTest(t)
	m.Handle(module.Touch{Note: 0, Touched: true})
	m.Handle(module.Expression{Channel: 2, Kind: "pressure", Value: 10})
	if len(h.MIDI) != 0 {
		t.Errorf("touch/expression sent %d messages, want 0", len(h.MIDI))
	}
}

// TestCloseReleasesHeldNotes covers both passthrough notes and mapped notes.
func TestCloseReleasesHeldNotes(t *testing.T) {
	m, h := newTest(t)
	m.overrides["note:40"] = MidiMapping{OutType: "note", OutCh: 1, OutNum: 45, OutMin: 0, OutMax: 127}

	m.Handle(module.Pad{Note: 36, Velocity: 100, Pressed: true}) // passthrough
	m.Handle(module.Pad{Note: 40, Velocity: 100, Pressed: true}) // mapped to 45
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
	if !offs[36] || !offs[45] {
		t.Errorf("Close released %v, want notes 36 (passthrough) and 45 (mapped)", offs)
	}
}

// TestSendFailuresAreCountedNotSwallowed mirrors thru's guard.
func TestSendFailuresAreCountedNotSwallowed(t *testing.T) {
	h := &moduletest.Host{NoMIDIOut: true}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatal(err)
	}
	m.Handle(module.Pad{Note: 36, Velocity: 100, Pressed: true})
	if m.lastErr == "" {
		t.Error("a failed send left lastErr empty")
	}
}

// TestDrawEmitsOnlySupportedOps also exercises the KVRows path, which no
// other module has used on real hardware yet.
func TestDrawEmitsOnlySupportedOps(t *testing.T) {
	m, h := newTest(t)
	m.overrides["note:40"] = MidiMapping{OutType: "note", OutCh: 1, OutNum: 45, OutMin: 0, OutMax: 127}
	m.Handle(module.Pad{Note: 40, Velocity: 100, Pressed: true})

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
	sawKVRows := false
	for _, op := range f.Ops() {
		if !supported[op.Kind] {
			t.Errorf("Draw emitted unsupported op %q", op.Kind)
		}
		if op.Kind == "kvrows" {
			sawKVRows = true
		}
	}
	if !sawKVRows {
		t.Error("Draw with an override present did not emit a kvrows op")
	}
}

// TestDrawTextIsASCII is the regression test for the actual bug found on
// hardware 2026-08-17: the empty-overrides message used an em-dash, which the
// host's sanitiser silently turned into "?" on the real screen. "Draw emitted
// only known op kinds" was passing the whole time — content, not just kind,
// has to be checked.
func TestDrawTextIsASCII(t *testing.T) {
	m, _ := newTest(t)
	f := module.NewFrame(960, 160)
	m.Draw(f) // the empty-overrides path is exactly where the bug was

	if bad := moduletest.NonASCIIStrings(f); len(bad) != 0 {
		t.Errorf("Draw emitted non-ASCII text: %q", bad)
	}

	m.overrides["note:40"] = MidiMapping{OutType: "note", OutCh: 1, OutNum: 45, OutMin: 0, OutMax: 127, Name: "test"}
	m.Handle(module.Pad{Note: 40, Velocity: 100, Pressed: true})
	f2 := module.NewFrame(960, 160)
	m.Draw(f2)
	if bad := moduletest.NonASCIIStrings(f2); len(bad) != 0 {
		t.Errorf("Draw with an override emitted non-ASCII text: %q", bad)
	}
}
