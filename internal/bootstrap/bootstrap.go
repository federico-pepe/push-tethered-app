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
	"github.com/federico-pepe/push-tethered-app/internal/midiin"
	"github.com/federico-pepe/push-tethered-app/internal/midiout"
	"github.com/federico-pepe/push-tethered-app/internal/mirror"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// Options configures what gets opened. Zero value is a reasonable default
// (display, LEDs and MIDI-out all enabled; 30fps; no capture) except for
// Modules, which has no sensible default and must be set.
type Options struct {
	FPS       int
	NoDisplay bool
	NoLEDs    bool

	// MIDIIn selects the input cable by driver port number — the only way to
	// address a specific unit when two Push units present identical port
	// names (see pmidi.OpenRef). Takes precedence over MIDIInName when set
	// (InName != "").
	MIDIIn pmidi.PortRef
	// MIDIInName is a MIDI port name, used only when MIDIIn is unset. Kept for
	// existing callers; empty auto-detects the Live port (see pmidi.Open),
	// which now refuses when more than one Push is attached rather than
	// guessing.
	MIDIInName string

	// DisplaySel selects the USB unit by display.Info.ID ("serial:..." or
	// "usb:BUS.ADDR", see display.List). Empty means display.Open's default:
	// the first unit found, Push 3 preferred.
	DisplaySel string

	MIDIOutName string
	NoMIDIOut   bool

	// ExtMIDIInName names the external MIDI input port (see internal/midiin) —
	// unrelated to MIDIIn/MIDIInName, which select Push's own cable. Empty
	// means midiin.DefaultName when creating; "the first non-Push port" when
	// attaching (Windows).
	ExtMIDIInName string
	NoExtMIDIIn   bool

	// ExtMIDIInFromPushExternal and ExtMIDIOutToPushExternal, when set,
	// route a module's ExternalMIDI/MIDI-out through Push 3's own External
	// Port cable — the physical MIDI DIN jacks — instead of through
	// midiin/midiout's virtual loopback port. Push 2 has no External Port
	// (see docs/protocol/midi-input.md); Open logs a warning and ignores
	// the flag rather than failing when the connected unit isn't a Push 3.
	// ExtMIDIInFromPushExternal takes precedence over ExtMIDIInName/
	// NoExtMIDIIn; ExtMIDIOutToPushExternal is independent of MIDIOutName.
	ExtMIDIInFromPushExternal bool
	ExtMIDIOutToPushExternal  bool

	CapturePath string
	CaptureRaw  bool

	// Mirror, when set, receives every rendered frame for live streaming —
	// see internal/mirror. The caller owns constructing it (and serving its
	// HTTP handler somewhere), since a multi-session caller like
	// cmd/pushapp-ui needs one Hub per session routed under a shared server,
	// which is not something Open can decide on the caller's behalf the way
	// it does for CapturePath's single output file.
	Mirror *mirror.Hub

	Modules []module.Module
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
	switch {
	case opts.MIDIIn.InName != "":
		port, err = pmidi.OpenRef(opts.MIDIIn)
	case opts.MIDIInName != "":
		port, err = pmidi.OpenNamed(opts.MIDIInName)
	default:
		port, err = pmidi.Open()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("MIDI: %w", err)
	}
	log.Printf("MIDI: connected to %q", port.Name())

	var dev *display.Device
	if !opts.NoDisplay {
		dev, err = display.OpenID(opts.DisplaySel)
		switch {
		case errors.Is(err, display.ErrBusy):
			// The documented degrade path: something else owns the screen
			// (usually Live with Push as a control surface). Keep the session.
			log.Printf("display: %v", err)
			log.Printf("display: continuing MIDI-only — quit Live to get the screen")
		case errors.Is(err, display.ErrAlreadyClaimed):
			// Not a degrade path: the caller asked for a unit this process
			// already drives, which is a bug in the caller, not a condition
			// to work around.
			port.Close()
			return nil, nil, fmt.Errorf("display: %w", err)
		case err != nil:
			port.Close()
			return nil, nil, fmt.Errorf("display: %w", err)
		default:
			log.Printf("display: claimed %s (%s)", dev.Model(), dev.Info().ID)
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
	openMIDIIn := func() (*midiin.In, error) { return midiin.Open(opts.ExtMIDIInName) }
	if opts.NoExtMIDIIn {
		openMIDIIn = nil
	}

	if opts.ExtMIDIInFromPushExternal || opts.ExtMIDIOutToPushExternal {
		extRef, ok := findExternalRef(port.Ref())
		if !ok {
			log.Printf("external MIDI: Push 3 External Port not found for %q (Push 2 has none) — falling back to virtual loopback port", port.Ref().Unit)
		} else {
			if opts.ExtMIDIInFromPushExternal {
				openMIDIIn = func() (*midiin.In, error) { return midiin.OpenExisting(extRef.InName, extRef.InNum) }
			}
			if opts.ExtMIDIOutToPushExternal {
				if extRef.OutNum < 0 {
					log.Printf("external MIDI: no output cable paired with %q — MIDI-out modules keep using the virtual loopback port", extRef.InName)
				} else {
					openMIDIOut = func() (*midiout.Out, error) { return midiout.OpenExisting(extRef.OutName, extRef.OutNum) }
				}
			}
		}
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

	// A nil *mirror.Hub must not become a non-nil host.FrameSink — an
	// interface wrapping a typed nil pointer is itself non-nil, which would
	// make host.Runtime.drawFrame call Frame on a nil Hub and panic.
	var mirrorSink host.FrameSink
	if opts.Mirror != nil {
		mirrorSink = opts.Mirror
	}

	rt, err = host.New(port, dev, host.Options{
		FPS:         opts.FPS,
		NoDisplay:   opts.NoDisplay,
		NoLEDs:      opts.NoLEDs,
		OpenMIDIOut: openMIDIOut,
		OpenMIDIIn:  openMIDIIn,
		Recorder:    rec,
		Mirror:      mirrorSink,
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

// findExternalRef looks up the External Port cable belonging to the same
// physical unit as the already-opened control-surface port. Push 2 units
// never produce one (docs/protocol/midi-input.md), so ok is false there.
func findExternalRef(mainRef pmidi.PortRef) (pmidi.PortRef, bool) {
	for _, ref := range pmidi.ListPortRefs() {
		if ref.Unit == mainRef.Unit && ref.Role == "External" {
			return ref, true
		}
	}
	return pmidi.PortRef{}, false
}

func closeHardware(dev *display.Device, port *pmidi.Port) {
	if dev != nil {
		dev.Close()
	}
	port.Close()
}
