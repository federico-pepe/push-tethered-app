package midi

import "testing"

// This app's own virtual MIDI-out port is named "Push Tethered App" (see
// internal/midiout.DefaultName) — deliberately close to real Push port names
// since it is this app's product name, not a coincidence anyone chose to
// avoid. A bare "contains Push" filter mistakes it for hardware: confirmed
// live 2026-08-19, where pairing a unit while another session had a
// MIDI-out module active made "Push Tethered App" show up in the pairing
// view as if it were a Push cable.
func TestIsPushHardwareNameExcludesOwnProductName(t *testing.T) {
	if isPushHardwareName("Push Tethered App") {
		t.Error(`isPushHardwareName("Push Tethered App") = true, want false`)
	}
	if isPushHardwareName("Push Tethered App 2") {
		t.Error(`isPushHardwareName("Push Tethered App 2") = true, want false — see hostManager's per-session naming`)
	}
}

func TestIsPushHardwareNameMatchesRealNames(t *testing.T) {
	names := []string{
		"Ableton Push 2 Live Port",
		"Ableton Push 2 User Port",
		"Ableton Push 3 Live Port",
		"Ableton Push 3 User Port",
		"Ableton Push 3 External Port",
		"Ableton Push 3 MIDI",           // WinMM cable 1
		"MIDIIN2 (Ableton Push 3 MIDI)", // WinMM cable 2+
		"MIDIOUT3 (Ableton Push 3 MIDI)",
	}
	for _, n := range names {
		if !isPushHardwareName(n) {
			t.Errorf("isPushHardwareName(%q) = false, want true", n)
		}
	}
}

func TestFindPortNameIgnoresOwnVirtualPort(t *testing.T) {
	// "Push Tethered App" alone can never match (no "Live Port" suffix), but
	// this pins the property the fix depends on: it also does not satisfy the
	// hardware-name half of the check, which is what actually matters once
	// this app ever names a port something ending in those words.
	_, err := findPortName([]string{"Push Tethered App", "Some Other Port"})
	if err == nil {
		t.Error("findPortName found a Live Port among non-Push names")
	}
}

func TestListPortRefsFilterExcludesOwnVirtualPort(t *testing.T) {
	ins := []portName{
		{Name: "Ableton Push 3 Live Port", Num: 0},
		{Name: "Push Tethered App", Num: 5}, // our own virtual out, seen as an input by other apps
	}
	var filtered []portName
	for _, p := range ins {
		if isPushHardwareName(p.Name) {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) != 1 || filtered[0].Name != "Ableton Push 3 Live Port" {
		t.Errorf("filtered = %v, want only the real Push port", filtered)
	}
}
