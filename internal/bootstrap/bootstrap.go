// Package bootstrap is the hardware-opening sequence shared by every entry
// point that runs the module host: claim MIDI, claim the display (degrading
// on ErrBusy rather than failing), wire up an optional MIDI-out opener and
// screen recorder, and build a *host.Runtime.
//
// It exists because cmd/pushapp-ui needs to do exactly what cmd/pushapp does
// before either can hand control to the host — and this is exactly the kind
// of logic (the ErrBusy degrade path, the "open lazily, not now" MIDI-out
// contract) that must not drift between two copies. One real second caller is
// the point at which that duplication actually costs something, so it moves
// here rather than staying inlined in cmd/pushapp/main.go alone.
package bootstrap

import (
	"errors"
	"fmt"
	"log"

	"github.com/federico-pepe/push-tethered-app/internal/capture"
	"github.com/federico-pepe/push-tethered-app/internal/display"
	"github.com/federico-pepe/push-tethered-app/internal/host"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
	"github.com/federico-pepe/push-tethered-app/internal/midiout"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// Options configures what gets opened. Zero value is a reasonable default
// (display, LEDs and MIDI-out all enabled; 30fps; no capture) except for
// Modules, which has no sensible default and must be set.
type Options struct {
	FPS         int
	NoDisplay   bool
	NoLEDs      bool
	MIDIInName  string // exact port name; empty auto-detects the Live port (see pmidi.Open)
	MIDIOutName string
	NoMIDIOut   bool
	CapturePath string
	CaptureRaw  bool
	Modules     []module.Module
}

// Open claims the hardware and returns a ready-to-run Runtime.
//
// On any failure it releases whatever it had already opened before returning
// — a caller that gets a non-nil error owns nothing and must not call
// cleanup. On success, the caller must arrange for cleanup to run after
// rt.Run returns and after rt.Shutdown(), mirroring cmd/pushapp's defer order:
// display before MIDI.
func Open(opts Options) (rt *host.Runtime, cleanup func(), err error) {
	if opts.FPS <= 0 {
		opts.FPS = 30
	}

	var port *pmidi.Port
	if opts.MIDIInName != "" {
		port, err = pmidi.OpenNamed(opts.MIDIInName)
	} else {
		port, err = pmidi.Open()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("MIDI: %w", err)
	}
	log.Printf("MIDI: connected to %q", port.Name())

	var dev *display.Device
	if !opts.NoDisplay {
		dev, err = display.Open()
		switch {
		case errors.Is(err, display.ErrBusy):
			// The documented degrade path: something else owns the screen
			// (usually Live with Push as a control surface). Keep the session.
			log.Printf("display: %v", err)
			log.Printf("display: continuing MIDI-only — quit Live to get the screen")
		case err != nil:
			port.Close()
			return nil, nil, fmt.Errorf("display: %w", err)
		default:
			log.Printf("display: claimed %s", dev.Model())
		}
	}

	// Handed to the host as an *opener*, not an open port. On macOS and Linux
	// opening one publishes it to the whole system, so it must happen only
	// when a module that actually sends MIDI is activated, not merely because
	// one is compiled in. The host owns the lifetime.
	openMIDIOut := func() (*midiout.Out, error) { return midiout.Open(opts.MIDIOutName) }
	if opts.NoMIDIOut {
		openMIDIOut = nil
	}

	// Taps the render output, so it costs no extra USB traffic and cannot
	// disturb what the panel shows.
	var rec capture.Recorder
	if opts.CapturePath != "" {
		rec, err = capture.New(capture.Options{Path: opts.CapturePath, FPS: opts.FPS, Raw: opts.CaptureRaw})
		if err != nil {
			closeHardware(dev, port)
			return nil, nil, fmt.Errorf("capture: %w", err)
		}
		mode := "panel-accurate"
		if opts.CaptureRaw {
			mode = "raw source"
		}
		log.Printf("capture: recording %s (%s)", rec.Path(), mode)
	}

	rt, err = host.New(port, dev, host.Options{
		FPS:         opts.FPS,
		NoDisplay:   opts.NoDisplay,
		NoLEDs:      opts.NoLEDs,
		OpenMIDIOut: openMIDIOut,
		Recorder:    rec,
	}, opts.Modules...)
	if err != nil {
		closeHardware(dev, port)
		return nil, nil, fmt.Errorf("host: %w", err)
	}

	// Process-loaded modules a previous run installed (see internal/host's
	// Install/Uninstall and internal/host/procmod). A scan failure here is
	// not fatal to starting the app — the compiled-in modules still work
	// regardless of whatever is wrong with the installed-modules directory.
	if err := rt.LoadInstalled(); err != nil {
		log.Printf("loading installed modules: %v", err)
	}

	return rt, func() { closeHardware(dev, port) }, nil
}

func closeHardware(dev *display.Device, port *pmidi.Port) {
	if dev != nil {
		dev.Close()
	}
	port.Close()
}
