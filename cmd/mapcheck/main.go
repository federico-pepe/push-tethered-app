// Command mapcheck cross-references captured Push MIDI against the button map
// in core/push3, and reports three things:
//
//   - CONFIRMED: a constant we have now seen on the wire, tethered
//   - UNKNOWN:   traffic on channel 1 that no constant accounts for
//   - UNSEEN:    constants not exercised by this capture (coverage gap)
//
// The names below are written here, but every *value* comes from core/push3 —
// so this verifies the shared map rather than duplicating it. If a constant
// changes upstream, this tool follows automatically.
//
// Input is tools/midimon.swift output (file argument, or stdin).
//
//	./midimon 60 | ./mapcheck
//	./mapcheck capture.txt
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
)

// ccNames maps CC number -> human name, valued from core/push3 constants.
var ccNames = map[byte]string{
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

// noteNames covers the non-pad notes on channel 1 (touch sensors). These come
// from internal/pushmap, not core/push3: the shared map's touch notes are off
// by one for encoders 1-8 and the volume wheel, and omit the touch strip.
// See internal/pushmap/touch.go and docs/feasibility.md §8.8.
var noteNames = pushmap.TouchNames()

var lineRe = regexp.MustCompile(`([0-9A-F]{2}(?: [0-9A-F]{2})*)\s*$`)

type seen struct {
	count  int
	values map[byte]int
}

func main() {
	in := os.Stdin
	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "opening %s: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}

	ccSeen := map[byte]*seen{}
	noteSeen := map[byte]*seen{}
	padSeen := map[byte]int{}
	mpeChans := map[int]int{}
	var unknownCC, unknownNote []byte

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		m := lineRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		var b []byte
		for _, h := range strings.Fields(m[1]) {
			v, err := strconv.ParseUint(h, 16, 8)
			if err != nil {
				break
			}
			b = append(b, byte(v))
		}
		if len(b) < 2 || b[0] >= 0xF8 { // skip realtime (Active Sensing etc.)
			continue
		}

		ch := int(b[0]&0x0F) + 1
		switch b[0] & 0xF0 {
		case 0xB0:
			if ch != 1 { // CC on ch2-16 is MPE expression, not the control surface
				mpeChans[ch]++
				continue
			}
			rec(ccSeen, b[1], b[2])
			if _, ok := ccNames[b[1]]; !ok {
				unknownCC = appendUniq(unknownCC, b[1])
			}
		case 0x90:
			if ch != 1 {
				mpeChans[ch]++
				continue
			}
			if push3.IsPadNote(b[1]) {
				padSeen[b[1]]++
				continue
			}
			rec(noteSeen, b[1], b[2])
			if _, ok := noteNames[b[1]]; !ok {
				unknownNote = appendUniq(unknownNote, b[1])
			}
		}
	}

	report(ccSeen, noteSeen, padSeen, mpeChans, unknownCC, unknownNote)
}

func rec(m map[byte]*seen, k, v byte) {
	s := m[k]
	if s == nil {
		s = &seen{values: map[byte]int{}}
		m[k] = s
	}
	s.count++
	s.values[v]++
}

func appendUniq(s []byte, v byte) []byte {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func report(ccSeen, noteSeen map[byte]*seen, padSeen map[byte]int, mpe map[int]int, unkCC, unkNote []byte) {
	fmt.Println("=== CONFIRMED (constant seen on the wire, tethered) ===")
	var conf []string
	for cc, s := range ccSeen {
		if n, ok := ccNames[cc]; ok {
			conf = append(conf, fmt.Sprintf("  CC %-4d %-22s n=%-4d %s", cc, n, s.count, valueSummary(s, cc)))
		}
	}
	for nt, s := range noteSeen {
		if n, ok := noteNames[nt]; ok {
			conf = append(conf, fmt.Sprintf("  Note %-2d %-22s n=%-4d", nt, n, s.count))
		}
	}
	sort.Strings(conf)
	if len(conf) == 0 {
		fmt.Println("  (none)")
	}
	for _, l := range conf {
		fmt.Println(l)
	}

	fmt.Println("\n=== UNKNOWN — channel 1 traffic no constant accounts for ===")
	if len(unkCC) == 0 && len(unkNote) == 0 {
		fmt.Println("  (none)")
	}
	sort.Slice(unkCC, func(i, j int) bool { return unkCC[i] < unkCC[j] })
	for _, cc := range unkCC {
		fmt.Printf("  CC %-4d  n=%-4d values=%v\n", cc, ccSeen[cc].count, sortedKeys(ccSeen[cc].values))
	}
	sort.Slice(unkNote, func(i, j int) bool { return unkNote[i] < unkNote[j] })
	for _, nt := range unkNote {
		fmt.Printf("  Note %-3d n=%-4d velocities=%v\n", nt, noteSeen[nt].count, sortedKeys(noteSeen[nt].values))
	}

	fmt.Println("\n=== UNSEEN — mapped but not exercised by this capture ===")
	var unseen []string
	for cc, n := range ccNames {
		if _, ok := ccSeen[cc]; !ok {
			unseen = append(unseen, fmt.Sprintf("  CC %-4d %s", cc, n))
		}
	}
	for nt, n := range noteNames {
		if _, ok := noteSeen[nt]; !ok {
			unseen = append(unseen, fmt.Sprintf("  Note %-2d %s", nt, n))
		}
	}
	sort.Strings(unseen)
	for _, l := range unseen {
		fmt.Println(l)
	}
	fmt.Printf("\ncoverage: %d/%d CC, %d/%d touch notes, %d/64 pads\n",
		countKnown(ccSeen, ccNames), len(ccNames),
		countKnown(noteSeen, noteNames), len(noteNames), len(padSeen))
	if len(mpe) > 0 {
		fmt.Printf("MPE traffic ignored on channels: %v\n", sortedIntKeys(mpe))
	}
}

// valueSummary annotates encoder CCs with decoded direction, so the
// CW/CCW question is answered from data rather than from the doc prose.
func valueSummary(s *seen, cc byte) string {
	vals := sortedKeys(s.values)
	if !push3.IsEncoderCC(cc) {
		return fmt.Sprintf("values=%v", vals)
	}
	var parts []string
	for _, v := range vals {
		parts = append(parts, fmt.Sprintf("%d->DecodeRel %+d (x%d)", v, push3.DecodeRel(v), s.values[v]))
	}
	return strings.Join(parts, " ")
}

func countKnown(seenM map[byte]*seen, names map[byte]string) int {
	n := 0
	for k := range seenM {
		if _, ok := names[k]; ok {
			n++
		}
	}
	return n
}

func sortedKeys(m map[byte]int) []byte {
	out := make([]byte, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedIntKeys(m map[int]int) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
