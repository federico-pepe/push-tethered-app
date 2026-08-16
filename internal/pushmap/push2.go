package pushmap

// Push 2 control-surface map, measured on hardware 2026-08-16 (§11).
//
// Most of Push 2 matches the core/push3 CC table exactly — screen rows, scene
// column, transport, modes, views, encoders. Only the entries below differ, so
// this file holds *deltas*, not a second full table. Use ButtonNameFor.
//
// Coverage from two ordered sweeps: 70/85 CC, 11/13 touch notes.

// Push 2 controls that core/push3's table does not describe correctly.
var push2Extra = map[byte]string{
	15:  "Swing encoder turn", // Push 3 has no Swing encoder
	52:  "Master",             // Push 3 uses CC 28 for "Select (main)"
	53:  "Stop Clip",          // Push 3 uses CC 29 for "Stop Clips"
	87:  "New",                // Push 3 uses CC 92 for New
	111: "Browse",             // Push 3 has no Browse button
}

// push2Absent lists Push 3 CCs that Push 2 has no control for. Not exhaustive —
// it covers the controls Push 3 gained (jog wheel, D-Pad centre, Set/Help/Save/
// Lock) rather than everything unswept.
var push2Absent = map[byte]bool{
	70: true, // jog wheel turn
	80: true, // Set
	81: true, // Help
	82: true, // Save
	83: true, // Lock
	91: true, // D-Pad centre
	92: true, // New (Push 2 uses 87)
	93: true, // jog click left
	94: true, // jog press
	95: true, // jog click right
}

// Device identifies which Push a name lookup is for.
type Device int

const (
	Push3 Device = iota // the default; core/push3's table applies as written
	Push2
)

func (d Device) String() string {
	if d == Push2 {
		return "Push 2"
	}
	return "Push 3"
}

// DeviceFromPortName infers the device from a MIDI port name such as
// "Ableton Push 2 Live Port". Defaults to Push 3.
func DeviceFromPortName(name string) Device {
	if containsFold(name, "push 2") {
		return Push2
	}
	return Push3
}

func containsFold(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	lower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + 32
		}
		return b
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if lower(s[i+j]) != lower(sub[j]) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ButtonNameFor returns the control name for a CC on the given device.
func ButtonNameFor(d Device, cc byte) (string, bool) {
	if d == Push2 {
		if n, ok := push2Extra[cc]; ok {
			return n, true
		}
		if push2Absent[cc] {
			return "", false
		}
	}
	return ButtonName(cc)
}

// Push2TouchNames are the touch sensors Push 2 has that Push 3 does not.
//
// Push 2's touch notes run contiguously 0-10: encoders 1-8 = 0-7, master volume
// = 8, **Swing = 9**, Tempo = 10, touch strip = 12. Push 3 dropped the Swing
// encoder and left note 9 unassigned — which is exactly the gap that made the
// upstream "encoders are notes 1-8" numbering look plausible (§8.8). The two
// devices explain each other.
var push2TouchExtra = map[byte]string{
	9: "Swing encoder touch",
}

// TouchNameFor returns the touch-sensor name for a note on the given device.
func TouchNameFor(d Device, note byte) (string, bool) {
	if d == Push2 {
		if n, ok := push2TouchExtra[note]; ok {
			return n, true
		}
	}
	return TouchName(note)
}

// IsRelativeEncoderCCFor reports whether a CC is a relative encoder on the
// given device. Push 2 adds the Swing encoder at CC 15 and has no jog wheel.
func IsRelativeEncoderCCFor(d Device, cc byte) bool {
	if d == Push2 {
		if cc == 15 {
			return true
		}
		if cc == 70 { // no jog wheel on Push 2
			return false
		}
	}
	return IsRelativeEncoderCC(cc)
}
