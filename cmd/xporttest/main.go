// Command xporttest runs a read-only, interleaved touch/release capture
// against Push 3's undocumented xPort interface (6), to test whether its
// traffic correlates with pad touch state.
//
// SAFETY: this tool NEVER writes to xPort — no OUT transfer, no control
// transfer, no SetAutoDetach. It claims interface 6 only; the display
// interface (0) is never touched. See CLAUDE.md's xPort rule and
// docs/protocol/xport.md.
//
// docs/protocol/xport.md's touch-correlation attempts were inconclusive
// twice over: first (2026-08-25), three separate time-blocked captures
// (idle, held, released) were confounded with the packet's own rotating
// content. Second (2026-08-26), fixed-round-count captures with a
// time.Sleep between reads turned out to silently drop data during every
// sleep — the marker's offset in each read drifted by up to ~1.1KB between
// consecutive reads, versus a measured ~136-byte marker-to-marker period
// within a read, proving reads were neither back-to-back nor frame-aligned.
//
// This version fixes both: it reads continuously with no sleep between
// reads (so nothing emitted by the device between reads is lost), tags
// each read with a wall-clock timestamp, and drives touch/release
// prompting off a wall-clock ticker independent of read count. Analysis
// then reassembles the continuous byte stream, locates the recurring
// FF FF FF 3F marker to find real frame boundaries (not read boundaries),
// and only after that does the per-offset, per-toggle touch/release
// consistency check.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/gousb"
)

const (
	vendorAbleton       = 0x2982
	productPush3        = 0x1969
	classVendorSpecific = 0xFF
	epXPortIn           = 0x84
	packetSize          = 512
	configNum           = 1
)

var frameMarker = []byte{0xFF, 0xFF, 0xFF, 0x3F}

func main() {
	duration := flag.Duration("duration", 20*time.Second, "total capture duration")
	phasePeriod := flag.Duration("phase-period", 2*time.Second, "wall-clock time per touch/release phase")
	margin := flag.Duration("margin", 400*time.Millisecond, "time to discard after each phase change, to absorb human reaction lag")
	readTimeout := flag.Duration("read-timeout", time.Second, "per-read timeout — only matters if the stream stalls")
	dumpPath := flag.String("dump", "", "optional path to write raw read events (elapsed-ms, hex)")
	flag.Parse()

	ctx := gousb.NewContext()
	defer ctx.Close()

	dev, ifaceNum, err := openXPort(ctx)
	if err != nil {
		log.Fatalf("opening xPort: %v", err)
	}
	defer dev.Close()

	cfg, err := dev.Config(configNum)
	if err != nil {
		log.Fatalf("selecting configuration %d: %v", configNum, err)
	}
	defer cfg.Close()

	intf, err := cfg.Interface(ifaceNum, 0)
	if err != nil {
		log.Fatalf("claiming interface %d: %v", ifaceNum, err)
	}
	defer intf.Close()

	in, err := intf.InEndpoint(epXPortIn)
	if err != nil {
		log.Fatalf("opening IN endpoint %#02x: %v", epXPortIn, err)
	}

	fmt.Printf("xPort (interface %d) claimed, read-only — this tool never writes to it.\n", ifaceNum)
	fmt.Println("Sit with a hand near the pad grid. Press Enter to start the capture.")
	bufio.NewReader(os.Stdin).ReadString('\n')

	var dumpFile *os.File
	if *dumpPath != "" {
		dumpFile, err = os.Create(*dumpPath)
		if err != nil {
			log.Fatalf("creating dump file: %v", err)
		}
		defer dumpFile.Close()
	}

	start := time.Now()
	deadline := start.Add(*duration)

	var wg sync.WaitGroup
	wg.Add(1)
	var events []readEvent
	go func() {
		defer wg.Done()
		events = captureStream(in, deadline, *readTimeout, start, dumpFile)
	}()

	changes := runPhasePrompts(*phasePeriod, deadline)
	wg.Wait()

	fmt.Printf("\ncaptured %d reads over %v, reassembling frames...\n", len(events), duration)
	frames := extractFrames(events)
	if len(frames) == 0 {
		fmt.Println("no frame marker (FF FF FF 3F) found in the captured stream — nothing to analyze")
		return
	}
	fmt.Printf("found %d frames (marker-aligned, %d bytes each)\n", len(frames), len(frames[0].data))

	data := tagPhases(frames, changes, *margin)
	fmt.Printf("%d frames survive the %v reaction-lag margin\n", len(data), *margin)
	analyze(data)
}

// readEvent is one completed bulk read, timestamped at completion.
type readEvent struct {
	ts   time.Time
	data []byte
}

// captureStream reads back-to-back with no pause between reads, so nothing
// the device emits between reads is dropped. It stops at deadline.
func captureStream(in *gousb.InEndpoint, deadline time.Time, readTimeout time.Duration, start time.Time, dumpFile *os.File) []readEvent {
	var events []readEvent
	for time.Now().Before(deadline) {
		buf := make([]byte, packetSize)
		rctx, cancel := context.WithTimeout(context.Background(), readTimeout)
		n, err := in.ReadContext(rctx, buf)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			log.Printf("read error: %v", err)
			continue
		}
		ts := time.Now()
		buf = buf[:n]
		events = append(events, readEvent{ts: ts, data: buf})
		if dumpFile != nil {
			fmt.Fprintf(dumpFile, "%d\t%x\n", ts.Sub(start).Milliseconds(), buf)
		}
	}
	return events
}

// phaseChange records a touch/release transition and when it happened.
type phaseChange struct {
	ts    time.Time
	touch bool
}

// runPhasePrompts alternates touch/release prompts on a wall-clock ticker,
// independent of how many reads happen in the meantime, and returns the
// timeline of phase changes for later tagging.
func runPhasePrompts(phasePeriod time.Duration, deadline time.Time) []phaseChange {
	fmt.Println("phase: RELEASE (hands off the pads)")
	changes := []phaseChange{{ts: time.Now(), touch: false}}
	touch := false
	ticker := time.NewTicker(phasePeriod)
	defer ticker.Stop()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return changes
		}
		select {
		case t := <-ticker.C:
			if t.After(deadline) {
				return changes
			}
			touch = !touch
			changes = append(changes, phaseChange{ts: t, touch: touch})
			if touch {
				fmt.Println(">>> TOUCH a pad now and hold <<<")
			} else {
				fmt.Println(">>> RELEASE now <<<")
			}
		case <-time.After(remaining):
			return changes
		}
	}
}

// frame is one marker-aligned slice of the reassembled byte stream.
type frame struct {
	ts   time.Time
	data []byte
}

// extractFrames concatenates every read in order, finds every occurrence of
// the recurring FF FF FF 3F marker, infers the true frame length from the
// most common marker-to-marker gap, and slices out one frame per marker —
// the alignment the raw 512-byte read boundaries never gave us.
func extractFrames(events []readEvent) []frame {
	var buf []byte
	var byteTS []time.Time
	for _, e := range events {
		buf = append(buf, e.data...)
		for range e.data {
			byteTS = append(byteTS, e.ts)
		}
	}

	var markerOffsets []int
	for i := 0; i+len(frameMarker) <= len(buf); i++ {
		if bytes.Equal(buf[i:i+len(frameMarker)], frameMarker) {
			markerOffsets = append(markerOffsets, i)
		}
	}
	if len(markerOffsets) < 2 {
		return nil
	}

	frameLen := modeGap(markerOffsets)
	if frameLen <= 0 {
		return nil
	}

	frames := make([]frame, 0, len(markerOffsets))
	for _, off := range markerOffsets {
		if off+frameLen > len(buf) {
			continue
		}
		frames = append(frames, frame{ts: byteTS[off], data: buf[off : off+frameLen]})
	}
	return frames
}

// modeGap returns the most common gap between consecutive offsets — the
// real frame period, resistant to the occasional larger gap caused by a
// read boundary or a stalled read.
func modeGap(offsets []int) int {
	counts := make(map[int]int)
	for i := 1; i < len(offsets); i++ {
		counts[offsets[i]-offsets[i-1]]++
	}
	best, bestCount := 0, 0
	for gap, count := range counts {
		if count > bestCount {
			best, bestCount = gap, count
		}
	}
	return best
}

// tagPhases assigns each frame the touch/release phase active at its
// timestamp, and drops frames captured within margin of a phase change —
// human reaction time to the printed prompt, not real signal.
func tagPhases(frames []frame, changes []phaseChange, margin time.Duration) []round {
	sort.Slice(changes, func(i, j int) bool { return changes[i].ts.Before(changes[j].ts) })

	data := make([]round, 0, len(frames))
	for _, f := range frames {
		touch := changes[0].touch
		changeTS := changes[0].ts
		for _, c := range changes {
			if c.ts.After(f.ts) {
				break
			}
			touch, changeTS = c.touch, c.ts
		}
		if f.ts.Sub(changeTS) < margin {
			continue
		}
		data = append(data, round{phase: touch, data: f.data})
	}
	return data
}

type round struct {
	phase bool // true = touch
	data  []byte
}

// segment is a maximal run of consecutive rounds sharing the same phase.
type segment struct {
	phase      bool
	start, end int // indices into data, end exclusive
}

func segmentsOf(data []round) []segment {
	var segs []segment
	for i, r := range data {
		if len(segs) == 0 || segs[len(segs)-1].phase != r.phase {
			segs = append(segs, segment{phase: r.phase, start: i, end: i + 1})
		} else {
			segs[len(segs)-1].end = i + 1
		}
	}
	return segs
}

func meanStd(vals []float64) (mean, std float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(len(vals))
	var ss float64
	for _, v := range vals {
		ss += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(ss / float64(len(vals)))
}

type finding struct {
	offset                 int
	meanTouch, meanRelease float64
	consistency            float64
	agree, total           int
}

// analyze reports byte offsets whose value reliably separates by touch state
// across every individual toggle, not just in aggregate — the check the
// 2026-08-25 attempt skipped, which is why it was fooled by the packet's own
// rotating content.
func analyze(data []round) {
	if len(data) == 0 {
		fmt.Println("no data captured")
		return
	}
	length := len(data[0].data)
	for _, r := range data {
		if len(r.data) < length {
			length = len(r.data)
		}
	}

	segs := segmentsOf(data)
	if len(segs) < 4 {
		fmt.Println("fewer than 4 phase segments survived — not enough touch/release toggles to test consistency; raise -duration or lower -phase-period")
		return
	}

	var findings []finding
	for o := 0; o < length; o++ {
		var touchAll, releaseAll []float64
		for _, s := range segs {
			for _, r := range data[s.start:s.end] {
				v := float64(r.data[o])
				if s.phase {
					touchAll = append(touchAll, v)
				} else {
					releaseAll = append(releaseAll, v)
				}
			}
		}
		mT, sT := meanStd(touchAll)
		mR, sR := meanStd(releaseAll)
		diff := mT - mR
		if math.Abs(diff) < 3 || math.Abs(diff) < (sT+sR)*1.2 {
			continue // not separated even in aggregate
		}

		agree, total := 0, 0
		for i, s := range segs {
			var neighborIdx = -1
			if i+1 < len(segs) {
				neighborIdx = i + 1
			} else if i > 0 {
				neighborIdx = i - 1
			}
			if neighborIdx < 0 || segs[neighborIdx].phase == s.phase {
				continue
			}
			n := segs[neighborIdx]

			segVals := make([]float64, 0, s.end-s.start)
			for _, r := range data[s.start:s.end] {
				segVals = append(segVals, float64(r.data[o]))
			}
			nVals := make([]float64, 0, n.end-n.start)
			for _, r := range data[n.start:n.end] {
				nVals = append(nVals, float64(r.data[o]))
			}
			segMean, _ := meanStd(segVals)
			nMean, _ := meanStd(nVals)

			local := segMean - nMean
			if !s.phase {
				local = -local // normalize to "touch minus release"
			}
			total++
			if (local > 0) == (diff > 0) {
				agree++
			}
		}

		if total == 0 {
			continue
		}
		consistency := float64(agree) / float64(total)
		if consistency < 0.7 {
			continue // fails per-toggle consistency — likely rotation, not touch
		}
		findings = append(findings, finding{
			offset: o, meanTouch: mT, meanRelease: mR,
			consistency: consistency, agree: agree, total: total,
		})
	}

	if len(findings) == 0 {
		fmt.Println("no byte offset showed a touch/release split that held up across individual toggles.")
		fmt.Println("either nobody touched a pad, the toggle cadence didn't line up with prompts, or region 1/3 aren't touch-encoded the way hypothesized.")
		return
	}

	fmt.Printf("%d offset(s) survive per-toggle consistency check (>=70%% agreement across %d segments):\n\n",
		len(findings), len(segs))
	fmt.Println("offset\tmean(touch)\tmean(release)\tdiff\tconsistency")
	for _, f := range findings {
		fmt.Printf("%d\t%.1f\t%.1f\t%+.1f\t%d/%d (%.0f%%)\n",
			f.offset, f.meanTouch, f.meanRelease, f.meanTouch-f.meanRelease,
			f.agree, f.total, f.consistency*100)
	}
}

// openXPort finds a Push 3 and identifies its xPort interface by name,
// without opening/claiming it yet. xPort is Push-3-only — Push 2 has no
// interface 6 (see docs/protocol/xport.md).
func openXPort(ctx *gousb.Context) (*gousb.Device, int, error) {
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == gousb.ID(vendorAbleton)
	})
	if err != nil && len(devs) == 0 {
		return nil, 0, err
	}
	if len(devs) == 0 {
		return nil, 0, fmt.Errorf("no Ableton device found (vendor %#04x) — is Push connected?", vendorAbleton)
	}

	var chosen *gousb.Device
	for _, d := range devs {
		if chosen == nil && d.Desc.Product == gousb.ID(productPush3) {
			chosen = d
			continue
		}
		d.Close()
	}
	if chosen == nil {
		return nil, 0, fmt.Errorf("xPort is Push-3-only — no Push 3 found among %d Ableton device(s)", len(devs))
	}

	cfg, ok := chosen.Desc.Configs[configNum]
	if !ok {
		chosen.Close()
		return nil, 0, fmt.Errorf("configuration %d not found in descriptor", configNum)
	}

	var candidates []int
	for _, iface := range cfg.Interfaces {
		for _, alt := range iface.AltSettings {
			if alt.Class != classVendorSpecific {
				continue
			}
			name, err := chosen.InterfaceDescription(cfg.Number, iface.Number, alt.Alternate)
			if err == nil && strings.Contains(strings.ToLower(name), "xport") {
				candidates = append(candidates, iface.Number)
			}
			break
		}
	}

	switch len(candidates) {
	case 0:
		chosen.Close()
		return nil, 0, errors.New(`no interface named "xPort" found on this device`)
	case 1:
		return chosen, candidates[0], nil
	default:
		chosen.Close()
		return nil, 0, fmt.Errorf("multiple interfaces named xPort: %v — refusing to guess", candidates)
	}
}
