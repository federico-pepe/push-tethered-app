package beatcount

import (
	"testing"

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

func TestDeclaresNeedsMIDIIn(t *testing.T) {
	if !New().Meta().NeedsMIDIIn {
		t.Error("beatcount must declare NeedsMIDIIn")
	}
}

// TestStartDrawsDigitOneTopToBottom pins the row mapping: digitBitmaps'
// written row 0 (top of the glyph as drawn in the source) must land on the
// physical top row (7), not the bottom — get this backwards and every digit
// renders upside down.
func TestStartDrawsDigitOneTopToBottom(t *testing.T) {
	m, h := newTest(t)
	h.Reset()

	m.Handle(module.ExternalMIDI{Raw: []byte{0xFA}}) // Start

	lit := h.LitPads()
	// digitBitmaps[0] (digit "1") row 0 is "00011000" — columns 3 and 4 —
	// and must land on physical row 7 (top), not row 0.
	top := push3.PadNote(3, 7)
	bottomWrong := push3.PadNote(3, 0)
	if lit[top] == 0 {
		t.Errorf("top of the glyph (col 3, physical row 7) is not lit — row mapping is backwards")
	}
	// Row 0 physically (bottom) is the glyph's base bar for "1"
	// (00011000 -> col 3,4 too, coincidentally), but row 1 (second from
	// bottom) is empty in the bitmap, so check a column that's only lit at
	// the top to catch an inversion unambiguously.
	_ = bottomWrong
	secondFromBottom := push3.PadNote(2, 1) // bitmap row 6 (second from top) has no col 2
	if lit[secondFromBottom] != 0 {
		t.Errorf("pad (col 2, physical row 1) is lit, want off — check row mapping")
	}
}

// TestClockAdvancesBeatEveryQuarterNote — beat only changes once a full
// quarter note's worth of ticks (ticksPerQuarterNote) has arrived.
func TestClockAdvancesBeatEveryQuarterNote(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.ExternalMIDI{Raw: []byte{0xFA}}) // Start, beat 0

	for i := 0; i < ticksPerQuarterNote-1; i++ {
		m.Handle(module.ExternalMIDI{Raw: []byte{0xF8}})
	}
	if m.beat != 0 {
		t.Errorf("beat = %d after %d ticks, want 0 (not yet a full quarter note)", m.beat, ticksPerQuarterNote-1)
	}

	m.Handle(module.ExternalMIDI{Raw: []byte{0xF8}}) // the ticksPerQuarterNote-th tick
	if m.beat != 1 {
		t.Errorf("beat = %d after a full quarter note, want 1", m.beat)
	}
}

// TestBeatWrapsAfterFourBeats — one bar of 4/4, then back to beat 0.
func TestBeatWrapsAfterFourBeats(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.ExternalMIDI{Raw: []byte{0xFA}})

	for beat := 0; beat < beats; beat++ {
		for i := 0; i < ticksPerQuarterNote; i++ {
			m.Handle(module.ExternalMIDI{Raw: []byte{0xF8}})
		}
	}
	if m.beat != 0 {
		t.Errorf("beat = %d after %d beats, want wrap to 0", m.beat, beats)
	}
}

// TestClockIgnoredBeforeStart — a clock arriving before any Start must not
// silently start counting from a half-initialised state; reset() (called by
// the first tick) is what onClock falls back to, and it must behave exactly
// like an explicit Start.
func TestClockBeforeStartActsLikeStart(t *testing.T) {
	m, h := newTest(t)
	h.Reset()

	m.Handle(module.ExternalMIDI{Raw: []byte{0xF8}})
	if !m.haveClock || m.beat != 0 {
		t.Errorf("first clock tick before Start: haveClock=%v beat=%d, want true/0", m.haveClock, m.beat)
	}
	if len(h.Pads) == 0 {
		t.Error("first clock tick before Start drew nothing")
	}
}
