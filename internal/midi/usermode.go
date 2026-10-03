package midi

import gm "gitlab.com/gomidi/midi/v2"

// Mode is the Push mode that SysEx command 0x0A selects: Live mode, where
// Live owns the pads and buttons, or User mode, where the User Port does.
//
// The bytes come from two independent sources:
//   - docs/protocol/midi-input.md: Push announces both toggles by itself, on
//     Live Port and User Port, when the user presses User.
//   - the constants in the Push 3 helper that ships inside Live 12
//     (Push2/sysex: MODE_SWITCH_MESSAGE_ID = 10, LIVE_MODE = 0, USER_MODE = 1).
//
// Whether the Push also ACCEPTS this message from the host is what
// plans/2026-10-03-user-mode-switch.md tests. Until a device test says so,
// nothing in the app may depend on it.
type Mode byte

const (
	LiveMode Mode = 0
	UserMode Mode = 1
)

// String names the mode for logs.
func (m Mode) String() string {
	switch m {
	case LiveMode:
		return "Live"
	case UserMode:
		return "User"
	}
	return "unknown"
}

// modeSwitchPrefix is F0, the Ableton manufacturer ID (00 21 1D), the device
// and model bytes (01 01, the same prefix push-manager uses for Push 3), and
// the command id 0x0A.
var modeSwitchPrefix = []byte{0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x0A}

// ModeSwitchSysEx returns the message that selects m:
// F0 00 21 1D 01 01 0A <mode> F7.
func ModeSwitchSysEx(m Mode) []byte {
	return append(append([]byte(nil), modeSwitchPrefix...), byte(m), 0xF7)
}

// ParseModeSwitch recognises a mode switch message, for example the
// announcement Push sends when the user presses User. ok is false for every
// other message, and for any mode value other than 0 and 1.
func ParseModeSwitch(b []byte) (m Mode, ok bool) {
	if len(b) != len(modeSwitchPrefix)+2 || b[len(b)-1] != 0xF7 {
		return 0, false
	}
	for i, v := range modeSwitchPrefix {
		if b[i] != v {
			return 0, false
		}
	}
	switch Mode(b[len(modeSwitchPrefix)]) {
	case LiveMode:
		return LiveMode, true
	case UserMode:
		return UserMode, true
	}
	return 0, false
}

// SendModeSwitch asks the Push to enter m. It sends exactly the fixed message
// from ModeSwitchSysEx and nothing else: there is deliberately no way to send
// other raw bytes through OutCable.
func (c *OutCable) SendModeSwitch(m Mode) error {
	return c.send(gm.Message(ModeSwitchSysEx(m)))
}

// SendModeSwitch asks the Push to enter m, on this port's paired output cable.
// Same fixed message as OutCable.SendModeSwitch. Callers must hold whatever
// lock guards the port's writes.
func (p *Port) SendModeSwitch(m Mode) error {
	return p.send(gm.Message(ModeSwitchSysEx(m)))
}

// Release closes the cable without clearing any pad, unlike Close. Use it
// when the cable carried only a mode switch.
func (c *OutCable) Release() {
	if c.out != nil && c.out.IsOpen() {
		_ = c.out.Close()
	}
}
