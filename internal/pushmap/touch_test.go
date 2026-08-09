package pushmap

import (
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

// TestMeasuredTouchNotes pins the values measured on tethered hardware
// (docs/feasibility.md §8.8). If these ever change, it should be because the
// hardware was re-measured — not because someone assumed a contiguous range.
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

// TestNote9Unused documents the gap the old contiguous-range assumption missed.
func TestNote9Unused(t *testing.T) {
	if name, ok := TouchName(9); ok {
		t.Errorf("note 9 should be unassigned, got %q", name)
	}
}

// TestDivergesFromCore is the point of this package: it fails if core/push3 is
// ever corrected upstream, which is the signal to delete this package and use
// core/push3 directly.
func TestDivergesFromCore(t *testing.T) {
	if push3.NoteEncoder1Touch == NoteEncoder1Touch &&
		push3.NoteVolumeTouch == NoteVolumeTouch {
		t.Fatal("core/push3 now agrees with the measured values — " +
			"fold pushmap into core/push3 and delete this package")
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
