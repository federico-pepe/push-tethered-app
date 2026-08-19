// Command identifytest drives internal/identify against real hardware,
// standalone: no bootstrap.Open, no host.Runtime, so it exercises Flash and
// FlashLEDs directly and can be pointed at either half of a two-Push rig.
//
// This exists because internal/identify's own tests are pixel and string
// assertions on paintMarker — the part that actually needs hardware, Push
// reclaiming the screen if writes stop and the marker being legible in
// practice, has no other way to check short of the full pairing UI in
// cmd/pushapp-ui, which does not exist yet (see
// plans/2026-08-19-multi-device.md, phases D5/D6).
//
//	go run ./cmd/identifytest -devices                          # see what's attached
//	go run ./cmd/identifytest -device serial:XXXX -label "A" -seconds 6
//	go run ./cmd/identifytest -out 0 -seconds 6                 # LEDs only, by out port number
package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/display"
	"github.com/federico-pepe/push-tethered-app/internal/identify"
	"github.com/federico-pepe/push-tethered-app/internal/midi"
)

func main() {
	listDevices := flag.Bool("devices", false, "list connected Push units and their MIDI ports, then exit")
	deviceSel := flag.String("device", "", "USB unit to flash on screen: serial:XXXX or usb:BUS.ADDR (empty: skip the display flash)")
	label := flag.String("label", "IDENTIFY", "ASCII label to draw on the flashed screen")
	outNum := flag.Int("out", -1, "MIDI out port number to flash LEDs on (from -devices; -1: skip the LED flash)")
	seconds := flag.Int("seconds", 6, "how long to flash")
	flag.Parse()
	log.SetFlags(0)

	if *listDevices {
		units, err := display.List()
		if err != nil {
			log.Fatalf("listing USB units: %v", err)
		}
		for _, u := range units {
			fmt.Println(u)
		}
		fmt.Println()
		for _, mu := range midi.ListUnits() {
			fmt.Printf("%s (%s)\n", mu.Key, mu.Device)
			for _, p := range mu.Ports {
				fmt.Printf("  cable %d %q: in #%d out #%d ambiguous=%v\n",
					p.Cable, p.InName, p.InNum, p.OutNum, p.Ambiguous)
			}
		}

		// groupPorts never guesses a pairing for an ambiguous cable, so its
		// OutNum is always -1 above — there is no automatic answer for which
		// raw output port belongs to which physical unit. This raw dump is
		// what a human (or a -out sweep) uses to find it by trial, e.g. via
		// FlashLEDs on each candidate in turn.
		fmt.Println("\nraw MIDI output ports (for manual -out disambiguation):")
		for _, name := range midi.ListOutPortNames() {
			fmt.Printf("  %s\n", name)
		}
		return
	}

	if *deviceSel == "" && *outNum < 0 {
		log.Fatal("nothing to do: pass -device, -out, or both (see -devices)")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	d := time.Duration(*seconds) * time.Second
	done := make(chan error, 2)
	running := 0

	if *deviceSel != "" {
		running++
		go func() {
			log.Printf("display: flashing %q on %s for %s", *label, *deviceSel, d)
			done <- identify.Flash(ctx, *deviceSel, *label, color.NRGBA{R: 255, G: 140, B: 0, A: 255}, d, 12)
		}()
	}
	if *outNum >= 0 {
		running++
		go func() {
			log.Printf("LEDs: flashing out port %d for %s", *outNum, d)
			// Palette index for a distinct, easy-to-spot colour — see
			// core/push3/colors.go; 21 is documented there as a real,
			// distinct "vivid indigo/purple-blue" on Push 3's queried table.
			done <- identify.FlashLEDs(ctx, *outNum, 21, d)
		}()
	}

	var firstErr error
	for i := 0; i < running; i++ {
		if err := <-done; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		log.Fatalf("%v", firstErr)
	}
	fmt.Println("done")
}
