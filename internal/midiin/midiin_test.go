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
		"push",
		"MIDIIN2 (Ableton Push 2)",
		"Push Tethered App In", // our own port name is matched too, deliberately —
		// see internal/midiout's TestIsPushGuard for the same on the output side.
	}
	for _, n := range push {
		if !isPush(n) {
			t.Errorf("isPush(%q) = false, want true", n)
		}
	}

	notPush := []string{
		"loopMIDI Port",
		"IAC Driver Bus 1",
	}
	for _, n := range notPush {
		if isPush(n) {
			t.Errorf("isPush(%q) = true, want false", n)
		}
	}
}
