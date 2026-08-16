// Command mapcheck cross-references captured Push MIDI against the button map
// in core/push3, and reports three things:
//
//   - CONFIRMED: a constant we have now seen on the wire, tethered
//   - UNKNOWN:   traffic on channel 1 that no constant accounts for
//   - UNSEEN:    constants not exercised by this capture (coverage gap)
//
// The name tables live in internal/pushmap and are shared with cmd/pushapp, so
// the two cannot drift. Every CC *value* in them comes from core/push3, so this
// verifies the shared map rather than duplicating it; touch notes come from
// pushmap's corrected table (§8.8).
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

// ccNames / noteNames annotate numbers with human names. They live in
// internal/pushmap so cmd/pushapp and this tool share one table instead of
// drifting apart, and are rebuilt per device once the capture reveals which
// Push produced it (Push 2 and Push 3 differ on a handful of controls, §11).
var (
	device    = pushmap.Push3
	ccNames   = map[byte]string{}
	noteNames = map[byte]string{}
)

// detectDevice scans a capture for the port name midimon prints on each line,
// so a Push 2 capture is annotated with Push 2's map without a flag.
func detectDevice(lines []string) pushmap.Device {
	for _, l := range lines {
		if strings.Contains(l, "Push") {
			if d := pushmap.DeviceFromPortName(l); d == pushmap.Push2 {
				return d
			}
		}
	}
	return pushmap.Push3
}

// buildTables fills ccNames/noteNames for the detected device.
func buildTables(d pushmap.Device) {
	for cc := 0; cc < 128; cc++ {
		if n, ok := pushmap.ButtonNameFor(d, byte(cc)); ok {
			ccNames[byte(cc)] = n
		}
	}
	for n := 0; n < 36; n++ {
		if name, ok := pushmap.TouchNameFor(d, byte(n)); ok {
			noteNames[byte(n)] = name
		}
	}
}

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
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	device = detectDevice(lines)
	buildTables(device)
	fmt.Printf("device: %s\n\n", device)

	for _, line := range lines {
		m := lineRe.FindStringSubmatch(line)
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
	if !pushmap.IsRelativeEncoderCCFor(device, cc) {
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
