package midiout

import "testing"

// TestStatusChannelConversion pins the 1-16 in / 0-15 on-the-wire conversion.
// The project talks about channels as 1-16 everywhere (docs, pushmap, the MPE
// notes), while MIDI encodes 0-15. Getting this backwards is silent: everything
// still sends, just one channel off.
func TestStatusChannelConversion(t *testing.T) {
	tests := []struct {
		kind, ch byte
		want     byte
	}{
		{0x90, 1, 0x90},  // note on, channel 1 -> nibble 0
		{0x90, 16, 0x9F}, // note on, channel 16 -> nibble F
		{0xB0, 1, 0xB0},  // CC, channel 1
		{0xB0, 2, 0xB1},  // CC, channel 2
		{0x80, 10, 0x89}, // note off, channel 10
	}
	for _, tt := range tests {
		got, err := status(tt.kind, tt.ch)
		if err != nil {
			t.Errorf("status(%#x, %d): unexpected error %v", tt.kind, tt.ch, err)
			continue
		}
		if got != tt.want {
			t.Errorf("status(%#x, %d) = %#x, want %#x", tt.kind, tt.ch, got, tt.want)
		}
	}
}

func TestStatusRejectsOutOfRangeChannel(t *testing.T) {
	for _, ch := range []byte{0, 17, 255} {
		if _, err := status(0x90, ch); err == nil {
			t.Errorf("status(0x90, %d): want error, got nil", ch)
		}
	}
}

// TestIsPushGuard is the feedback-loop guard. Attaching our output to Push's own
// port would send every module note straight back into the decoder.
func TestIsPushGuard(t *testing.T) {
	push := []string{
		"Ableton Push 3 Live Port",
		"Ableton Push 2 User Port",
		"MIDIIN2 (Ableton Push 2)",
	}
	for _, n := range push {
		if !isPush(n) {
			t.Errorf("isPush(%q) = false, want true", n)
		}
	}

	notPush := []string{
		"loopMIDI Port",
		"IAC Driver Bus 1",
		"push",
		// our own port name must NOT be caught: DefaultName is "Push Tethered
		// App", and a Windows user names their loopback port to match it —
		// self-excluding here would make the default name unattachable.
		"Push Tethered App",
	}
	for _, n := range notPush {
		if isPush(n) {
			t.Errorf("isPush(%q) = true, want false", n)
		}
	}
}
