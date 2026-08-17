// Command pushapp owns an Ableton Push 2 or Push 3 in tethered mode and runs
// modules on it.
//
// A module draws Push's screen and handles its pads, encoders and buttons.
// This binary is the host: it claims the display, reads the control surface,
// drives the LEDs, optionally owns a MIDI output port for modules that send to
// other software, and hands all of it to whichever module is active. It contains
// no UI logic of its own — see internal/module for the contract and
// plans/2026-08-17-module-host.md for the design.
//
// Ableton Live is not involved. If Live happens to be holding the display we
// degrade to a MIDI-only session and say so, rather than failing.
//
//	go run ./cmd/pushapp
//	go run ./cmd/pushapp -module monitor -fps 60
//	go run ./cmd/pushapp -list
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/federico-pepe/push-tethered-app/internal/bootstrap"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/modules/monitor"
	"github.com/federico-pepe/push-tethered-app/modules/remap"
	"github.com/federico-pepe/push-tethered-app/modules/seq"
	"github.com/federico-pepe/push-tethered-app/modules/thru"
)

// available lists the modules compiled into this binary.
//
// Explicit rather than an init()-time registry: the set is small, the order is
// the order the UI will show, and a test can build a Runtime with a different
// set. Out-of-process modules will be discovered at runtime and appended here
// once the process loader lands.
func available() []module.Module {
	return []module.Module{
		monitor.New(),
		thru.New(),
		seq.New(),
		remap.New(),
	}
}

func main() {
	fps := flag.Int("fps", 30, "display refresh rate")
	modID := flag.String("module", "", "module to run (default: the first one)")
	listMods := flag.Bool("list", false, "list available modules and exit")
	noDisplay := flag.Bool("no-display", false, "skip the display, run MIDI only")
	noLEDs := flag.Bool("no-leds", false, "do not drive LEDs")
	midiOutName := flag.String("midi-out", "", "MIDI output port to create, or attach to on Windows")
	noMIDIOut := flag.Bool("no-midi-out", false, "do not open a MIDI output port")
	capturePath := flag.String("capture", "", "record the screen to a file (.mp4, .mov or .gif)")
	captureRaw := flag.Bool("capture-raw", false, "record the source image instead of panel-accurate BGR565 colour")
	flag.Parse()

	log.SetFlags(0)

	mods := available()
	if *listMods {
		for _, m := range mods {
			meta := m.Meta()
			fmt.Printf("%-12s %s", meta.ID, meta.Name)
			if meta.NeedsMIDIOut {
				fmt.Print("  [needs MIDI out]")
			}
			fmt.Println()
		}
		return
	}

	rt, cleanup, err := bootstrap.Open(bootstrap.Options{
		FPS:         *fps,
		NoDisplay:   *noDisplay,
		NoLEDs:      *noLEDs,
		MIDIOutName: *midiOutName,
		NoMIDIOut:   *noMIDIOut,
		CapturePath: *capturePath,
		CaptureRaw:  *captureRaw,
		Modules:     mods,
	})
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer cleanup()

	if *modID != "" {
		if err := rt.Activate(*modID); err != nil {
			log.Fatalf("host: %v (see -list)", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runErr := rt.Run(ctx)
	fmt.Println()
	rt.Shutdown()
	if runErr != nil {
		log.Fatalf("host: %v", runErr)
	}
}
