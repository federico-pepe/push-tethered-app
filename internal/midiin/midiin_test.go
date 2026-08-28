package midiin

import "testing"

// TestIsPushGuard mirrors internal/midiout's — attaching to Push's own port
// would receive our own output right back (via midiout) or, worse here,
// silently duplicate Push's own control-surface traffic into a module's
// ExternalMIDI stream, which internal/midi already delivers through the
// normal Pad/Button/Encoder path.
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
		// our own port name must NOT be caught — see internal/midiout's
		// TestIsPushGuard for the same reasoning on the output side.
		"Push Tethered App In",
	}
	for _, n := range notPush {
		if isPush(n) {
			t.Errorf("isPush(%q) = true, want false", n)
		}
	}
}
