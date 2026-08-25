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
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/federico-pepe/push-tethered-app/internal/applog"
	"github.com/federico-pepe/push-tethered-app/internal/bootstrap"
	"github.com/federico-pepe/push-tethered-app/internal/display"
	"github.com/federico-pepe/push-tethered-app/internal/host/procmod"
	"github.com/federico-pepe/push-tethered-app/internal/midi"
	"github.com/federico-pepe/push-tethered-app/internal/mirror"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/version"
	"github.com/federico-pepe/push-tethered-app/modules/beatcount"
	"github.com/federico-pepe/push-tethered-app/modules/monitor"
	"github.com/federico-pepe/push-tethered-app/modules/paddebug"
	"github.com/federico-pepe/push-tethered-app/modules/padpointer"
	"github.com/federico-pepe/push-tethered-app/modules/remap"
	"github.com/federico-pepe/push-tethered-app/modules/seq"
	"github.com/federico-pepe/push-tethered-app/modules/thru"
	uitextdemo "github.com/federico-pepe/push-tethered-app/modules/ui-text-demo"
	"github.com/federico-pepe/push-tethered-app/modules/uidemo"
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
		beatcount.New(),
		uidemo.New(),
		uitextdemo.New(),
		paddebug.New(),
		padpointer.New(),
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
	extMIDIInName := flag.String("ext-midi-in", "", "external MIDI input port to create, or attach to on Windows — for modules that declare NeedsMIDIIn, e.g. to sync to an external clock")
	noExtMIDIIn := flag.Bool("no-ext-midi-in", false, "do not open an external MIDI input port")
	capturePath := flag.String("capture", "", "record the screen to a file (.mp4, .mov or .gif)")
	captureRaw := flag.Bool("capture-raw", false, "record the source image instead of panel-accurate BGR565 colour")
	mirrorAddr := flag.String("mirror-addr", "localhost:3000", "serve a live MJPEG mirror of the screen at http://<addr>/screen; pass -mirror-addr=\"\" to disable it. Avoid :7000/:5000 — macOS's AirPlay Receiver squats both by default.")
	installDir := flag.String("install", "", "install the module directory at this path (manifest.json + executable), then exit")
	uninstallID := flag.String("uninstall", "", "uninstall the process-loaded module with this id, then exit")
	listDevices := flag.Bool("devices", false, "list connected Push units and their MIDI ports, then exit")
	deviceSel := flag.String("device", "", "USB unit to drive: serial:XXXX or usb:BUS.ADDR (default: the first one, see -devices)")
	midiInName := flag.String("midi-in", "", "MIDI input port name to use (default: auto-detect the Live port; required if more than one Push is attached)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	log.SetFlags(0)
	log.SetOutput(applog.Wrap(os.Stderr))
	applog.Banner()

	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	// Both are pure filesystem operations — no hardware touched, no Push
	// needed connected, so they run before bootstrap.Open ever claims MIDI
	// or the display.
	if *installDir != "" {
		man, err := procmod.Install(*installDir)
		if err != nil {
			applog.Fatalf("%v", err)
		}
		fmt.Printf("installed %q (%s)\n", man.ID, man.Name)
		return
	}
	if *uninstallID != "" {
		if err := procmod.Uninstall(*uninstallID); err != nil {
			applog.Fatalf("%v", err)
		}
		fmt.Printf("uninstalled %q\n", *uninstallID)
		return
	}

	// Read-only: opens a handle per USB unit just long enough to read its
	// serial, and lists MIDI ports without opening any of them. Claims
	// nothing, so this is safe to run with Live open and with another
	// pushapp session already driving a unit — this is what a user should
	// paste into a bug report.
	if *listDevices {
		units, err := display.List()
		if err != nil {
			applog.Fatalf("listing USB units: %v", err)
		}
		if len(units) == 0 {
			fmt.Println("no Push units found on USB")
		}
		for _, u := range units {
			fmt.Println(u)
		}

		fmt.Println()
		midiUnits := midi.ListUnits()
		if len(midiUnits) == 0 {
			fmt.Println("no Push MIDI ports found")
		}
		for _, mu := range midiUnits {
			fmt.Printf("%s (%s)\n", mu.Key, mu.Device)
			for _, p := range mu.Ports {
				role := p.Role
				if role == "" {
					role = "unknown role"
				}
				status := fmt.Sprintf("in #%d -> out #%d", p.InNum, p.OutNum)
				if p.Ambiguous {
					status = "AMBIGUOUS — matches another unit's cable; cannot pair automatically"
				} else if p.OutNum < 0 {
					status = fmt.Sprintf("in #%d -> no output cable found", p.InNum)
				}
				// IsLive is only worth calling out separately when the role
				// string didn't already say so — the WinMM case, where Role
				// is always "" and cable position is the only signal.
				live := ""
				if p.IsLive && p.Role != "Live" {
					live = ", Live"
				}
				fmt.Printf("  cable %d (%s%s): %q  %s\n", p.Cable, role, live, p.InName, status)
			}
		}
		return
	}

	mods := available()
	if *listMods {
		for _, m := range mods {
			meta := m.Meta()
			fmt.Printf("%-12s %s", meta.ID, meta.Name)
			if meta.NeedsMIDIOut {
				fmt.Print("  [needs MIDI out]")
			}
			if meta.NeedsMIDIIn {
				fmt.Print("  [needs MIDI in]")
			}
			fmt.Println()
		}
		if installed, err := procmod.ListInstalled(); err == nil {
			for _, man := range installed {
				fmt.Printf("%-12s %s  [installed]", man.ID, man.Name)
				if man.NeedsMIDIOut {
					fmt.Print(", needs MIDI out")
				}
				if man.NeedsMIDIIn {
					fmt.Print(", needs MIDI in")
				}
				fmt.Println()
			}
		}
		return
	}

	var mirrorHub *mirror.Hub
	if *mirrorAddr != "" {
		mirrorHub = mirror.NewHub()
		mux := http.NewServeMux()
		mux.Handle("/screen", mirrorHub)
		go func() {
			if err := http.ListenAndServe(*mirrorAddr, mux); err != nil {
				log.Printf("mirror: %v — live mirror unavailable", err)
			}
		}()
		log.Printf("mirror: serving http://%s/screen", *mirrorAddr)
	}

	rt, cleanup, err := bootstrap.Open(bootstrap.Options{
		FPS:           *fps,
		NoDisplay:     *noDisplay,
		NoLEDs:        *noLEDs,
		MIDIInName:    *midiInName,
		DisplaySel:    *deviceSel,
		MIDIOutName:   *midiOutName,
		NoMIDIOut:     *noMIDIOut,
		ExtMIDIInName: *extMIDIInName,
		NoExtMIDIIn:   *noExtMIDIIn,
		CapturePath:   *capturePath,
		CaptureRaw:    *captureRaw,
		Mirror:        mirrorHub,
		Modules:       mods,
	})
	if err != nil {
		applog.Fatalf("%v", err)
	}
	defer cleanup()

	if *modID != "" {
		if err := rt.Activate(*modID); err != nil {
			applog.Fatalf("host: %v (see -list)", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runErr := rt.Run(ctx)
	fmt.Println()
	rt.Shutdown()
	if runErr != nil {
		applog.Fatalf("host: %v", runErr)
	}
}
