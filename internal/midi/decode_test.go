package midi

import (
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
)

// TestActiveSensingIsFiltered is the single most important decode test.
//
// Push sends 0xFE about 37 times a second — over half of all traffic. The trap:
// 0xFE & 0xF0 == 0xF0, so masking the status byte before testing for system
// realtime makes keepalive decode as SysEx. The guard is testing >= 0xF8 first,
// and this pins it.
func TestActiveSensingIsFiltered(t *testing.T) {
	for _, b := range []byte{0xF8, 0xFA, 0xFC, 0xFE, 0xFF} {
		if ev := Decode([]byte{b, 0x00, 0x00}); ev != nil {
			t.Errorf("Decode(%#x) = %T, want nil (system realtime must be dropped)", b, ev)
		}
	}
}

// TestChannelIsDecodedBeforeCC pins the collision that makes MPE dangerous:
// CC 71 and CC 74 are encoder 1 and encoder 4 on channel 1, and are also MPE
// timbre controllers on a note's member channel. The numbers collide; only the
// channel disambiguates. Decoding CC before channel turns pad slide into
// phantom encoder movement.
func TestChannelIsDecodedBeforeCC(t *testing.T) {
	// Channel 1, CC 74 -> encoder 4.
	ev := Decode([]byte{0xB0, 74, 1})
	enc, ok := ev.(Encoder)
	if !ok {
		t.Fatalf("ch1 CC 74 = %T, want Encoder", ev)
	}
	if enc.Index != 3 {
		t.Errorf("ch1 CC 74 index = %d, want 3 (encoder 4)", enc.Index)
	}

	// Channel 2, CC 74 -> MPE slide, NOT an encoder.
	ev = Decode([]byte{0xB1, 74, 64})
	exp, ok := ev.(Expression)
	if !ok {
		t.Fatalf("ch2 CC 74 = %T, want Expression", ev)
	}
	if exp.Kind != "slide" {
		t.Errorf("ch2 CC 74 kind = %q, want \"slide\"", exp.Kind)
	}
	if exp.Channel != 2 {
		t.Errorf("ch2 CC 74 channel = %d, want 2", exp.Channel)
	}
}

// TestCC15AndCC111PerDevice covers the two CCs that mean different things on the
// two devices. Resolving these device-agnostically is a real bug: on Push 2,
// CC 15 is the Swing encoder, so decoding it as a button turns every turn into
// phantom presses.
func TestCC15AndCC111PerDevice(t *testing.T) {
	// CC 15: Push 2 Swing encoder / Push 3 Tempo press.
	if ev := DecodeFor(pushmap.Push2, []byte{0xB0, 15, 1}); !isEncoder(ev) {
		t.Errorf("Push 2 CC 15 = %T, want Encoder (Swing)", ev)
	}
	if ev := DecodeFor(pushmap.Push3, []byte{0xB0, 15, 127}); !isButton(ev) {
		t.Errorf("Push 3 CC 15 = %T, want Button (Tempo press)", ev)
	}

	// CC 111: Push 2 Browse button / Push 3 Volume encoder press.
	if ev := DecodeFor(pushmap.Push2, []byte{0xB0, 111, 127}); !isButton(ev) {
		t.Errorf("Push 2 CC 111 = %T, want Button (Browse)", ev)
	}
	if ev := DecodeFor(pushmap.Push3, []byte{0xB0, 111, 127}); !isButton(ev) {
		t.Errorf("Push 3 CC 111 = %T, want Button (Volume press)", ev)
	}
}

// TestJogWheelIsAnEncoderOnPush3Only guards a bug that already happened once:
// CC 70 is a relative encoder, and decoding it as a button turns every jog turn
// into a stream of phantom button presses.
//
// It is Push 3 only — Push 2 has no jog wheel, and CC 70 is listed in
// pushmap.push2Absent. The device-scoped predicate exists for exactly this kind
// of asymmetry, so both halves are asserted.
func TestJogWheelIsAnEncoderOnPush3Only(t *testing.T) {
	ev := DecodeFor(pushmap.Push3, []byte{0xB0, push3.CCJogWheel, 1})
	enc, ok := ev.(Encoder)
	if !ok {
		t.Fatalf("Push 3 CC 70 = %T, want Encoder", ev)
	}
	if enc.Index != -1 {
		t.Errorf("jog index = %d, want -1 (not one of the eight screen encoders)", enc.Index)
	}
	if enc.Delta != 1 {
		t.Errorf("jog delta = %d, want 1", enc.Delta)
	}

	// Push 2 must not decode CC 70 as an encoder. It should never arrive at all;
	// if it does, an unnamed button is the honest answer rather than a phantom
	// jog wheel the hardware does not have.
	if ev := DecodeFor(pushmap.Push2, []byte{0xB0, push3.CCJogWheel, 1}); isEncoder(ev) {
		t.Errorf("Push 2 CC 70 decoded as an Encoder; Push 2 has no jog wheel")
	}
}

// TestEncoderDirectionAndAcceleration pins two's-complement decoding and the
// fact that one message is not one click. The direction was documented backwards
// upstream at one point, so it is worth an explicit assertion.
func TestEncoderDirectionAndAcceleration(t *testing.T) {
	tests := []struct {
		val  byte
		want int
		what string
	}{
		{1, 1, "one click clockwise"},
		{127, -1, "one click counter-clockwise"},
		{11, 11, "fast clockwise turn"},
		{117, -11, "fast counter-clockwise turn"},
	}
	for _, tt := range tests {
		ev := Decode([]byte{0xB0, push3.CCEncoder1, tt.val})
		enc, ok := ev.(Encoder)
		if !ok {
			t.Fatalf("%s: got %T, want Encoder", tt.what, ev)
		}
		if enc.Delta != tt.want {
			t.Errorf("%s: value %d -> delta %d, want %+d", tt.what, tt.val, enc.Delta, tt.want)
		}
	}
}

// TestPadCornersAndCoordinates checks both measured corners and the orientation.
// Row 0 is the bottom row; getting the flip wrong renders the grid upside down,
// which is exactly the class of bug that looked fine in the logs.
func TestPadCornersAndCoordinates(t *testing.T) {
	tests := []struct {
		note     byte
		col, row int
		what     string
	}{
		{36, 0, 0, "bottom-left"},
		{99, 7, 7, "top-right"},
		{43, 7, 0, "bottom-right"},
		{92, 0, 7, "top-left"},
	}
	for _, tt := range tests {
		ev := Decode([]byte{0x90, tt.note, 100})
		pad, ok := ev.(Pad)
		if !ok {
			t.Fatalf("%s (note %d) = %T, want Pad", tt.what, tt.note, ev)
		}
		if pad.Col != tt.col || pad.Row != tt.row {
			t.Errorf("%s (note %d) = col %d row %d, want col %d row %d",
				tt.what, tt.note, pad.Col, pad.Row, tt.col, tt.row)
		}
		if !pad.Pressed {
			t.Errorf("%s: velocity 100 note-on decoded as release", tt.what)
		}
	}
}

// TestNoteOnZeroVelocityIsRelease — the running-status convention. Treating it
// as a press leaves pads lit forever.
func TestNoteOnZeroVelocityIsRelease(t *testing.T) {
	ev := Decode([]byte{0x90, 36, 0})
	pad, ok := ev.(Pad)
	if !ok {
		t.Fatalf("got %T, want Pad", ev)
	}
	if pad.Pressed {
		t.Error("note-on with velocity 0 decoded as a press, want release")
	}

	if ev := Decode([]byte{0x80, 36, 64}); !isReleasedPad(ev) {
		t.Errorf("explicit note-off = %#v, want a released Pad", ev)
	}
}

// TestMPEPadsArriveOnMemberChannels — MPE is on by default but not always, and
// the trigger is unidentified. Both layouts must decode as pads.
func TestMPEPadsArriveOnMemberChannels(t *testing.T) {
	for ch := byte(0); ch < 16; ch++ {
		ev := Decode([]byte{0x90 | ch, 36, 100})
		pad, ok := ev.(Pad)
		if !ok {
			t.Errorf("pad note-on on channel %d = %T, want Pad", ch+1, ev)
			continue
		}
		if pad.Channel != int(ch)+1 {
			t.Errorf("channel = %d, want %d", pad.Channel, ch+1)
		}
	}
}

// TestTouchNotes covers the sensors, including the off-by-one that was corrected
// upstream and the note-9 asymmetry between devices.
func TestTouchNotes(t *testing.T) {
	ev := Decode([]byte{0x90, 0, 127})
	touch, ok := ev.(Touch)
	if !ok {
		t.Fatalf("note 0 = %T, want Touch (encoder 1)", ev)
	}
	if !touch.Touched {
		t.Error("velocity 127 on a touch note decoded as release")
	}

	// Note 12 is the touch strip; its position arrives separately as pitch bend.
	if ev := Decode([]byte{0x90, 12, 127}); !isTouch(ev) {
		t.Errorf("note 12 = %T, want Touch (touch strip)", ev)
	}

	// Note 9 is unused on Push 3 and the Swing encoder touch on Push 2. Either
	// way it must decode as a Touch rather than being dropped — an unnamed
	// sensor is still a sensor.
	if ev := DecodeFor(pushmap.Push2, []byte{0x90, 9, 127}); !isTouch(ev) {
		t.Errorf("Push 2 note 9 = %T, want Touch (Swing encoder)", ev)
	}
}

// TestPitchBendIsBend — the touch strip reports position this way.
func TestPitchBendIsBend(t *testing.T) {
	// Channel 2 pitch bend, 14-bit little-endian-ish: lsb | msb<<7.
	ev := Decode([]byte{0xE1, 0x00, 0x40})
	exp, ok := ev.(Expression)
	if !ok {
		t.Fatalf("got %T, want Expression", ev)
	}
	if exp.Kind != "bend" {
		t.Errorf("kind = %q, want \"bend\"", exp.Kind)
	}
	if exp.Value != 8192 {
		t.Errorf("centre bend = %d, want 8192", exp.Value)
	}
}

// TestTruncatedAndJunkMessagesAreDropped — a short read must not index past the
// end of the slice.
func TestTruncatedAndJunkMessagesAreDropped(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0xB0},          // status only
		{0xB0, 71},      // CC with no value
		{0x90, 36},      // note-on with no velocity
		{0xE1, 0x00},    // pitch bend, one byte short
		{0x00, 0x00, 0}, // not a status byte at all
		{0xC0, 1, 1},    // program change: real MIDI, but nothing Push sends
	}
	for _, b := range cases {
		if ev := DecodeFor(pushmap.Push3, b); ev != nil {
			t.Errorf("Decode(% X) = %T, want nil", b, ev)
		}
	}
}

func isEncoder(ev Event) bool { _, ok := ev.(Encoder); return ok }
func isButton(ev Event) bool  { _, ok := ev.(Button); return ok }
func isTouch(ev Event) bool   { _, ok := ev.(Touch); return ok }

func isReleasedPad(ev Event) bool {
	p, ok := ev.(Pad)
	return ok && !p.Pressed
}
