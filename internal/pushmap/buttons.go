package pushmap

import "github.com/federico-pepe/ableton-push-hack/core/push3"

// buttonNames maps CC number -> human name. The names are written here but
// every *value* is imported from core/push3, so this annotates the shared map
// rather than duplicating it: if a constant changes upstream, this follows.
//
// Unlike the touch notes (see touch.go), these CC values are NOT known to be
// wrong — only 8 of them have been exercised on tethered hardware so far.
// cmd/mapcheck's UNSEEN list tracks the remaining coverage gap.
var buttonNames = map[byte]string{
	push3.CCScreenTop1: "Screen top 1", push3.CCScreenTop2: "Screen top 2",
	push3.CCScreenTop3: "Screen top 3", push3.CCScreenTop4: "Screen top 4",
	push3.CCScreenTop5: "Screen top 5", push3.CCScreenTop6: "Screen top 6",
	push3.CCScreenTop7: "Screen top 7", push3.CCScreenTop8: "Screen top 8",

	push3.CCScreenBot1: "Screen bottom 1", push3.CCScreenBot2: "Screen bottom 2",
	push3.CCScreenBot3: "Screen bottom 3", push3.CCScreenBot4: "Screen bottom 4",
	push3.CCScreenBot5: "Screen bottom 5", push3.CCScreenBot6: "Screen bottom 6",
	push3.CCScreenBot7: "Screen bottom 7", push3.CCScreenBot8: "Screen bottom 8",

	push3.CCEncoder1: "Encoder 1 turn", push3.CCEncoder2: "Encoder 2 turn",
	push3.CCEncoder3: "Encoder 3 turn", push3.CCEncoder4: "Encoder 4 turn",
	push3.CCEncoder5: "Encoder 5 turn", push3.CCEncoder6: "Encoder 6 turn",
	push3.CCEncoder7: "Encoder 7 turn", push3.CCEncoder8: "Encoder 8 turn",
	push3.CCVolume: "Volume wheel turn", push3.CCTempo: "Tempo wheel turn",

	push3.CCJogWheel: "Jog wheel turn", push3.CCJogPress: "Jog press",
	push3.CCJogClickLeft: "Jog click left", push3.CCJogClickRight: "Jog click right",

	push3.CCDPadUp: "D-Pad up", push3.CCDPadRight: "D-Pad right",
	push3.CCDPadDown: "D-Pad down", push3.CCDPadLeft: "D-Pad left",
	push3.CCDPadCenter: "D-Pad center",

	push3.CCSet: "Set", push3.CCSettings: "Settings",
	push3.CCHelp: "Help", push3.CCUserMode: "User Mode",

	push3.CCDeviceView: "Device View", push3.CCMixerView: "Mixer View",
	push3.CCClipView: "Clip View", push3.CCSessionView: "Session View",

	push3.CCShift: "Shift", push3.CCSelect: "Select",

	push3.CCUndo: "Undo", push3.CCSave: "Save",
	push3.CCAdd: "Add", push3.CCSwap: "Swap",

	push3.CCLock: "Lock", push3.CCStopClips: "Stop Clips",
	push3.CCMute: "Mute", push3.CCSolo: "Solo", push3.CCSelectMain: "Select (main)",

	push3.CCTapTempo: "Tap Tempo", push3.CCMetronome: "Metronome",
	push3.CCQuantize: "Quantize", push3.CCFixedLength: "Fixed Length",
	push3.CCAutomate: "Automate", push3.CCNew: "New",
	push3.CCCapture: "Capture", push3.CCRecord: "Record", push3.CCPlay: "Play",

	push3.CCScene14: "Scene 1/4", push3.CCScene14t: "Scene 1/4t",
	push3.CCScene18: "Scene 1/8", push3.CCScene18t: "Scene 1/8t",
	push3.CCScene116: "Scene 1/16", push3.CCScene116t: "Scene 1/16t",
	push3.CCScene132: "Scene 1/32", push3.CCScene132t: "Scene 1/32t",

	push3.CCRepeat: "Repeat", push3.CCAccent: "Accent", push3.CCScale: "Scale",
	push3.CCLayout: "Layout", push3.CCNote: "Note", push3.CCSession: "Session",

	push3.CCDoubleLoop: "Double Loop", push3.CCDuplicate: "Duplicate",
	push3.CCConvert: "Convert", push3.CCDelete: "Delete",

	push3.CCOctaveUp: "Octave Up", push3.CCOctaveDown: "Octave Down",
	push3.CCPageLeft: "Page Left", push3.CCPageRight: "Page Right",
}

// ButtonName returns the name for a CC, and whether it is mapped.
func ButtonName(cc byte) (string, bool) {
	n, ok := buttonNames[cc]
	return n, ok
}

// ButtonNames returns a copy of the CC name table.
func ButtonNames() map[byte]string {
	out := make(map[byte]string, len(buttonNames))
	for k, v := range buttonNames {
		out[k] = v
	}
	return out
}

// LitButtons returns the CCs worth driving as LEDs — the two screen rows.
// Used to clear button LEDs on shutdown without blasting all 85 CCs, some of
// which are encoders rather than buttons.
func LitButtons() []byte {
	out := make([]byte, 0, 16)
	for i := 0; i < 8; i++ {
		out = append(out, push3.CCScreenTopN(i), push3.CCScreenBotN(i))
	}
	return out
}
