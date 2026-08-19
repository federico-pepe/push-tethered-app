package midi

import "testing"

func TestUnitKeyOf(t *testing.T) {
	tests := []struct {
		name      string
		wantUnit  string
		wantRole  string
		wantCable int
	}{
		// CoreMIDI / ALSA: the role lives in the string itself.
		{"Ableton Push 3 Live Port", "Ableton Push 3", "Live", 1},
		{"Ableton Push 3 User Port", "Ableton Push 3", "User", 2},
		{"Ableton Push 3 External Port", "Ableton Push 3", "External", 3},
		{"Ableton Push 2 Live Port", "Ableton Push 2", "Live", 1},
		{"Ableton Push 2 User Port", "Ableton Push 2", "User", 2},

		// WinMM: no jack strings at all. First cable is bare; the rest are
		// wrapped with a MIDIIN/MIDIOUT prefix and a number that is NOT the
		// same as the role — see docs/platform/windows.md.
		{"Ableton Push 3 MIDI", "Ableton Push 3 MIDI", "", 1},
		{"MIDIIN2 (Ableton Push 3 MIDI)", "Ableton Push 3 MIDI", "", 2},
		{"MIDIIN3 (Ableton Push 3 MIDI)", "Ableton Push 3 MIDI", "", 3},
		{"MIDIOUT2 (Ableton Push 3 MIDI)", "Ableton Push 3 MIDI", "", 2},

		// Unrecognised shape: degrade to "one unnamed cable" rather than fail.
		{"something else entirely", "something else entirely", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unit, role, cable := unitKeyOf(tt.name)
			if unit != tt.wantUnit || role != tt.wantRole || cable != tt.wantCable {
				t.Errorf("unitKeyOf(%q) = (%q, %q, %d), want (%q, %q, %d)",
					tt.name, unit, role, cable, tt.wantUnit, tt.wantRole, tt.wantCable)
			}
		})
	}
}

func in(name string, num int) portName  { return portName{Name: name, Num: num} }
func out(name string, num int) portName { return portName{Name: name, Num: num} }

func findRef(t *testing.T, refs []PortRef, inName string) PortRef {
	t.Helper()
	for _, r := range refs {
		if r.InName == inName {
			return r
		}
	}
	t.Fatalf("no ref for %q among %d refs", inName, len(refs))
	return PortRef{}
}

// --- Single-unit cases: these are measured, real port lists. ---

func TestGroupPortsCoreMIDISingleUnit(t *testing.T) {
	ins := []portName{
		in("Ableton Push 3 Live Port", 0),
		in("Ableton Push 3 User Port", 1),
	}
	outs := []portName{
		out("Ableton Push 3 Live Port", 0),
		out("Ableton Push 3 User Port", 1),
	}
	refs := groupPorts(ins, outs)
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}

	live := findRef(t, refs, "Ableton Push 3 Live Port")
	if !live.IsLive || live.Ambiguous || live.OutName != "Ableton Push 3 Live Port" {
		t.Errorf("Live ref = %+v", live)
	}
	user := findRef(t, refs, "Ableton Push 3 User Port")
	if user.IsLive || user.Ambiguous || user.OutName != "Ableton Push 3 User Port" {
		t.Errorf("User ref = %+v", user)
	}
}

func TestGroupPortsWinMMSingleUnit(t *testing.T) {
	ins := []portName{
		in("Ableton Push 3 MIDI", 0),
		in("MIDIIN2 (Ableton Push 3 MIDI)", 1),
		in("MIDIIN3 (Ableton Push 3 MIDI)", 2),
	}
	outs := []portName{
		out("Ableton Push 3 MIDI", 0),
		out("MIDIOUT2 (Ableton Push 3 MIDI)", 1),
		out("MIDIOUT3 (Ableton Push 3 MIDI)", 2),
	}
	refs := groupPorts(ins, outs)
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}

	// Cable 1 has no literal out-name match ("Ableton Push 3 MIDI" happens to
	// match exactly here, which is the easy case); assert the harder ones,
	// where in and out names never share a literal string.
	cable2 := findRef(t, refs, "MIDIIN2 (Ableton Push 3 MIDI)")
	if cable2.OutName != "MIDIOUT2 (Ableton Push 3 MIDI)" || cable2.OutNum != 1 {
		t.Errorf("cable 2 = %+v, want paired with MIDIOUT2", cable2)
	}
	if cable2.IsLive {
		t.Errorf("cable 2 must not be treated as Live — only cable 1 is, on WinMM")
	}

	cable1 := findRef(t, refs, "Ableton Push 3 MIDI")
	if !cable1.IsLive {
		t.Errorf("cable 1 must be treated as Live on WinMM, where role strings do not exist")
	}
}

// --- Two-unit cases: HYPOTHESES, not measurements.
//
// Nobody has attached two identical Push units and recorded what CoreMIDI,
// ALSA or WinMM actually name their ports — see plans/2026-08-19-multi-device.md
// open question 2. These assert the *shape* the ambiguity fallback must take
// under the worst plausible case (both units producing byte-identical cable
// names), so that if the real behaviour turns out kinder than this, only these
// tests need loosening — not the production code. A failure here on real data
// means our guess was wrong, not that groupPorts regressed.

func TestGroupPortsTwoUnitsIdenticalNamesFallBackToAmbiguous(t *testing.T) {
	// Worst case: two units enumerate with byte-identical names and the OS
	// gives no way to tell them apart except port number.
	ins := []portName{
		in("Ableton Push 3 Live Port", 0),
		in("Ableton Push 3 Live Port", 2),
	}
	outs := []portName{
		out("Ableton Push 3 Live Port", 0),
		out("Ableton Push 3 Live Port", 2),
	}
	refs := groupPorts(ins, outs)
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	for _, r := range refs {
		if !r.Ambiguous {
			t.Errorf("ref for in-port %d not marked Ambiguous: %+v", r.InNum, r)
		}
		if r.OutNum != -1 {
			t.Errorf("ambiguous ref for in-port %d must not carry a guessed OutNum, got %d", r.InNum, r.OutNum)
		}
	}
}

// If the two units happen to enumerate with distinguishable names — e.g. two
// different macOS-assigned suffixes, which is plausible but unmeasured — they
// must group and pair independently rather than fall into the ambiguous path
// meant for the identical-name case.
func TestGroupPortsTwoUnitsDistinguishableNamesGroupIndependently(t *testing.T) {
	ins := []portName{
		in("Ableton Push 3 (1) Live Port", 0),
		in("Ableton Push 3 (2) Live Port", 2),
	}
	outs := []portName{
		out("Ableton Push 3 (1) Live Port", 0),
		out("Ableton Push 3 (2) Live Port", 2),
	}
	refs := groupPorts(ins, outs)
	for _, r := range refs {
		if r.Ambiguous {
			t.Errorf("ref %+v marked Ambiguous despite distinguishable names", r)
		}
	}
	first := findRef(t, refs, "Ableton Push 3 (1) Live Port")
	if first.OutName != "Ableton Push 3 (1) Live Port" {
		t.Errorf("first unit paired with %q, want its own out port", first.OutName)
	}
	second := findRef(t, refs, "Ableton Push 3 (2) Live Port")
	if second.OutName != "Ableton Push 3 (2) Live Port" {
		t.Errorf("second unit paired with %q, want its own out port", second.OutName)
	}
}

// Only the input side colliding is enough to force ambiguity for that cable —
// pairing it against either identical-looking output would be a guess.
func TestGroupPortsAmbiguousInputsDoNotBorrowAnOutput(t *testing.T) {
	ins := []portName{
		in("Ableton Push 3 Live Port", 0),
		in("Ableton Push 3 Live Port", 2),
	}
	outs := []portName{
		out("Ableton Push 3 Live Port", 0),
	}
	refs := groupPorts(ins, outs)
	for _, r := range refs {
		if !r.Ambiguous || r.OutNum != -1 {
			t.Errorf("ref %+v should be ambiguous with no output assigned", r)
		}
	}
}

// A non-Push port sharing an ambiguous unit key must not contaminate an
// otherwise unambiguous pairing for a different cable of the same units.
func TestGroupPortsAmbiguityIsPerCableNotPerUnit(t *testing.T) {
	ins := []portName{
		in("Ableton Push 3 Live Port", 0), // unit A, ambiguous with unit B below
		in("Ableton Push 3 Live Port", 2), // unit B
		in("Ableton Push 3 User Port", 1), // only unit A has a User Port
	}
	outs := []portName{
		out("Ableton Push 3 Live Port", 0),
		out("Ableton Push 3 Live Port", 2),
		out("Ableton Push 3 User Port", 1),
	}
	refs := groupPorts(ins, outs)
	user := findRef(t, refs, "Ableton Push 3 User Port")
	if user.Ambiguous {
		t.Errorf("User Port ref wrongly marked Ambiguous: %+v", user)
	}
	if user.OutName != "Ableton Push 3 User Port" {
		t.Errorf("User Port ref not paired: %+v", user)
	}
}

func TestListUnitsGroupsByUnit(t *testing.T) {
	refs := []PortRef{
		{InName: "Ableton Push 3 Live Port", Unit: "Ableton Push 3"},
		{InName: "Ableton Push 3 User Port", Unit: "Ableton Push 3"},
		{InName: "Ableton Push 2 Live Port", Unit: "Ableton Push 2"},
	}
	byKey := map[string][]PortRef{}
	for _, r := range refs {
		byKey[r.Unit] = append(byKey[r.Unit], r)
	}
	if len(byKey["Ableton Push 3"]) != 2 {
		t.Errorf("expected 2 cables grouped under Push 3, got %d", len(byKey["Ableton Push 3"]))
	}
	if len(byKey["Ableton Push 2"]) != 1 {
		t.Errorf("expected 1 cable grouped under Push 2, got %d", len(byKey["Ableton Push 2"]))
	}
}
