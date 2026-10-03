package midi

import (
	"bytes"
	"testing"
)

func TestModeSwitchSysExBytes(t *testing.T) {
	for _, c := range []struct {
		mode Mode
		want []byte
	}{
		{UserMode, []byte{0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x0A, 0x01, 0xF7}},
		{LiveMode, []byte{0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x0A, 0x00, 0xF7}},
	} {
		if got := ModeSwitchSysEx(c.mode); !bytes.Equal(got, c.want) {
			t.Errorf("%v: got % X, want % X", c.mode, got, c.want)
		}
	}
}

func TestModeSwitchSysExDoesNotShareStorage(t *testing.T) {
	a := ModeSwitchSysEx(UserMode)
	a[7] = 0x7F // a caller scribbling on its copy must not change the next message
	if b := ModeSwitchSysEx(UserMode); b[7] != 0x01 {
		t.Errorf("the prefix was modified: % X", b)
	}
}

func TestParseModeSwitchRoundTrip(t *testing.T) {
	for _, m := range []Mode{LiveMode, UserMode} {
		got, ok := ParseModeSwitch(ModeSwitchSysEx(m))
		if !ok || got != m {
			t.Errorf("%v: got %v ok=%v", m, got, ok)
		}
	}
}

func TestParseModeSwitchRejectsEverythingElse(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":              nil,
		"no end":             {0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x0A, 0x01},
		"mode 2":             {0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x0A, 0x02, 0xF7},
		"other command":      {0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x3A, 0x01, 0xF7},
		"other manufacturer": {0xF0, 0x00, 0x21, 0x1E, 0x01, 0x01, 0x0A, 0x01, 0xF7},
		"too long":           {0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x0A, 0x01, 0x00, 0xF7},
		"live heartbeat":     {0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x3A, 0x22, 0x64, 0xF7},
	} {
		if m, ok := ParseModeSwitch(b); ok {
			t.Errorf("%s: accepted as %v", name, m)
		}
	}
}

func TestDecodeTurnsOnlyTheModeAnnouncementIntoAnEvent(t *testing.T) {
	if ev, ok := Decode(ModeSwitchSysEx(UserMode)).(ModeChange); !ok || ev.Mode != UserMode {
		t.Errorf("User announcement: got %#v", Decode(ModeSwitchSysEx(UserMode)))
	}
	if ev, ok := Decode(ModeSwitchSysEx(LiveMode)).(ModeChange); !ok || ev.Mode != LiveMode {
		t.Errorf("Live announcement: got %#v", Decode(ModeSwitchSysEx(LiveMode)))
	}
	// Live's own recurring SysEx and an identity reply must stay undecoded.
	for _, b := range [][]byte{
		{0xF0, 0x00, 0x21, 0x1D, 0x01, 0x01, 0x3A, 0x22, 0x64, 0xF7},
		{0xF0, 0x7E, 0x01, 0x06, 0x02, 0x00, 0x21, 0x1D, 0x69, 0xF7},
	} {
		if ev := Decode(b); ev != nil {
			t.Errorf("% X decoded as %#v", b, ev)
		}
	}
}
