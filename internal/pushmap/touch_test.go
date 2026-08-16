package pushmap

import (
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

// TestMeasuredTouchNotes pins the values measured on tethered hardware
// (docs/feasibility.md §8.8, §12). These now come from core/push3, which was
// corrected upstream on 2026-08-16 — this test guards against a regression
// there, since a silent revert would be hard to spot from this side.
func TestMeasuredTouchNotes(t *testing.T) {
	want := map[byte]string{
		0: "Encoder 1 touch", 1: "Encoder 2 touch", 2: "Encoder 3 touch",
		3: "Encoder 4 touch", 4: "Encoder 5 touch", 5: "Encoder 6 touch",
		6: "Encoder 7 touch", 7: "Encoder 8 touch",
		8: "Volume wheel touch", 10: "Tempo wheel touch",
		11: "Jog wheel touch", 12: "Touch strip touch", 13: "D-Pad center touch",
	}
	for note, name := range want {
		got, ok := TouchName(note)
		if !ok {
			t.Errorf("note %d: not known, want %q", note, name)
			continue
		}
		if got != name {
			t.Errorf("note %d: got %q, want %q", note, got, name)
		}
	}
}

// TestNote9UnusedOnPush3 documents the gap that the old contiguous-range
// assumption missed. Note 9 is the Swing encoder on Push 2 (see push2_test).
func TestNote9UnusedOnPush3(t *testing.T) {
	if name, ok := TouchName(9); ok {
		t.Errorf("note 9 should be unassigned on Push 3, got %q", name)
	}
	if name, ok := TouchNameFor(Push2, 9); !ok || name != "Swing encoder touch" {
		t.Errorf("note 9 on Push 2 = (%q, %v), want Swing encoder touch", name, ok)
	}
}

// TestEncoderTouchNote checks the 0-indexed helper across all eight encoders.
func TestEncoderTouchNote(t *testing.T) {
	for n := 0; n < 8; n++ {
		if got, want := EncoderTouchNote(n), byte(n); got != want {
			t.Errorf("EncoderTouchNote(%d) = %d, want %d", n, got, want)
		}
	}
}

// TestJogIsEncoder guards the upstream fix: CC 70 must decode as a relative
// encoder, not a button. Getting this wrong produces an endless stream of
// phantom button presses, because both 1 and 127 are non-zero (§9.4).
func TestJogIsEncoder(t *testing.T) {
	if !push3.IsEncoderCC(push3.CCJogWheel) {
		t.Error("push3.IsEncoderCC(CCJogWheel) = false, want true")
	}
	if !IsRelativeEncoderCCFor(Push3, push3.CCJogWheel) {
		t.Error("jog wheel should be a relative encoder on Push 3")
	}
	if IsRelativeEncoderCCFor(Push2, push3.CCJogWheel) {
		t.Error("Push 2 has no jog wheel; CC 70 should not be an encoder there")
	}
}

// TestCC15PerDevice is the sharpest reason lookups are device-scoped: the same
// CC is a relative encoder on one device and a push-button on the other (§12.3).
func TestCC15PerDevice(t *testing.T) {
	if !IsRelativeEncoderCCFor(Push2, 15) {
		t.Error("CC 15 on Push 2 is the Swing encoder, want encoder")
	}
	if IsRelativeEncoderCCFor(Push3, 15) {
		t.Error("CC 15 on Push 3 is the tempo encoder PRESS, want not-encoder")
	}
	if n, _ := ButtonNameFor(Push2, 15); n != "Swing encoder turn" {
		t.Errorf("Push 2 CC 15 = %q", n)
	}
	if n, _ := ButtonNameFor(Push3, 15); n != "Tempo encoder press" {
		t.Errorf("Push 3 CC 15 = %q", n)
	}
	if n, _ := ButtonNameFor(Push2, 111); n != "Browse" {
		t.Errorf("Push 2 CC 111 = %q", n)
	}
	if n, _ := ButtonNameFor(Push3, 111); n != "Volume encoder press" {
		t.Errorf("Push 3 CC 111 = %q", n)
	}
}
