// Command frametest claims Push's vendor-specific display interface and pushes
// a single rendered frame to bulk OUT endpoint 0x01.
//
// This is the Phase 1 experiment from docs/archive/feasibility.md: does the display
// protocol observed *inside* a standalone Push 3 also work over USB in
// controller mode? A lit screen is a more definitive answer than a bus capture.
//
// The test image is drawn entirely with the sibling ableton-push-hack core/
// packages (gfx, gfx/text, gfx/widgets) — so a successful run also proves that
// the whole existing Push screen toolkit renders correctly over this transport,
// not just that some bytes reached the panel.
//
// Safety: claims interface 0 only. Interfaces 1-5 (audio, MIDI) stay bound to
// the OS class drivers. Nothing is ever written to interface 6 ("xPort"),
// which is vendor-specific and undocumented.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"time"

	coredisplay "github.com/federico-pepe/ableton-push-hack/core/display"
	"github.com/federico-pepe/ableton-push-hack/core/gfx"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/google/gousb"
)

const (
	vendorAbleton = 0x2982
	pidPush3      = 0x1969

	displayIface = 0    // "Ableton Push 3 Display", vendor-specific 255/255/255
	displayEPNum = 1    // bulk OUT 0x01
	configNum    = 1    // device has exactly one configuration
)

// frameHeader precedes every display frame. Confirmed identical on Push 2
// (official spec) and Push 3 (push_hook.c:17).
var frameHeader = []byte{
	0xFF, 0xCC, 0xAA, 0x88,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

// xorPattern is the line shaping applied to pixel data — 0xFFE7F3E7 seen
// phase-shifted by one byte (push_hook.c:194).
var xorPattern = [4]byte{0xE7, 0xF3, 0xE7, 0xFF}

func main() {
	var (
		double  = flag.Bool("double", false, "send the frame twice (655360B) as the standalone device does; default sends one frame")
		noXOR   = flag.Bool("no-xor", false, "skip the XOR shaping — expect garbage, useful to confirm XOR is required")
		fps     = flag.Int("fps", 30, "frames per second in loop mode")
		seconds = flag.Int("seconds", 10, "how long to hold the screen; 0 sends a single frame and exits")
	)
	flag.Parse()

	ctx := gousb.NewContext()
	defer ctx.Close()

	dev, err := ctx.OpenDeviceWithVIDPID(vendorAbleton, pidPush3)
	if err != nil {
		log.Fatalf("opening Push 3 (%#04x:%#04x): %v", vendorAbleton, pidPush3, err)
	}
	if dev == nil {
		log.Fatalf("Push 3 not found — connected and in controller mode?")
	}
	defer dev.Close()

	// NOTE: deliberately NOT calling dev.SetAutoDetach(true).
	//
	// gousb's autodetach is config-wide, not interface-wide: Device.Config()
	// loops over every interface in the configuration and detaches each one.
	// On this device that would tear the audio (1-3) and MIDI (4-5) interfaces
	// away from the OS class drivers — exactly what co-existence mode must not
	// do. It also fails outright on macOS with LIBUSB_ERROR_ACCESS, since
	// there is no detachable kernel driver to speak of.
	//
	// Interface 0 is vendor-specific with no class driver bound, so on Linux
	// there should be nothing to detach either. If a Linux run ever reports
	// LIBUSB_ERROR_BUSY here, detach interface 0 alone rather than enabling
	// autodetach.

	cfg, err := dev.Config(configNum)
	if err != nil {
		log.Fatalf("selecting configuration %d: %v", configNum, err)
	}
	defer cfg.Close()

	intf, err := cfg.Interface(displayIface, 0)
	if err != nil {
		log.Fatalf("claiming interface %d (is Live running and holding the display?): %v", displayIface, err)
	}
	defer intf.Close()
	log.Printf("claimed %s", intf)

	ep, err := intf.OutEndpoint(displayEPNum)
	if err != nil {
		log.Fatalf("opening OUT endpoint %#02x: %v", displayEPNum, err)
	}

	// sendFrame renders phase, encodes it and writes header + pixels.
	sendFrame := func(phase int) error {
		img := renderTestImage(*double, *noXOR, phase)

		// ToBGR565 emits TotalBytes: the frame duplicated into both halves.
		full := coredisplay.ToBGR565(img)
		payload := full[:push3.FrameBytes]
		if *double {
			payload = full
		}
		if !*noXOR {
			applyXOR(payload)
		}

		wctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := ep.WriteContext(wctx, frameHeader); err != nil {
			return fmt.Errorf("frame header: %w", err)
		}
		if _, err := ep.WriteContext(wctx, payload); err != nil {
			return fmt.Errorf("pixels: %w", err)
		}
		return nil
	}

	if *seconds == 0 {
		if err := sendFrame(0); err != nil {
			log.Fatalf("sending frame: %v", err)
		}
		log.Printf("single frame sent — Push's own idle UI will likely redraw over it")
		return
	}

	// Push keeps redrawing its own screen, so a single frame only flashes.
	// Holding the display means continuously outrunning the device's renderer.
	interval := time.Second / time.Duration(*fps)
	deadline := time.Now().Add(time.Duration(*seconds) * time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("holding display for %ds at %d fps (%s/frame, %.1f MB/s)...",
		*seconds, *fps, interval,
		float64(push3.FrameBytes**fps)/(1024*1024))

	start := time.Now()
	frames := 0
	for range ticker.C {
		if err := sendFrame(frames); err != nil {
			log.Fatalf("frame %d: %v", frames, err)
		}
		frames++
		if time.Now().After(deadline) {
			break
		}
	}

	elapsed := time.Since(start)
	log.Printf("sent %d frames in %s — %.1f fps actual, %.1f MB/s",
		frames, elapsed.Round(time.Millisecond),
		float64(frames)/elapsed.Seconds(),
		float64(frames*push3.FrameBytes)/elapsed.Seconds()/(1024*1024))

	fmt.Println("\nDone — Push's own UI should have taken the screen back now.")
	fmt.Println("  held steady, marker moving -> protocol fully confirmed at speed")
	fmt.Println("  flickering/fighting         -> device UI still redrawing; try -fps 60")
	fmt.Println("  colours wrong               -> channel order differs from the standalone path")
}

// applyXOR applies the line-shaping pattern to px in place.
func applyXOR(px []byte) {
	for i := range px {
		px[i] ^= xorPattern[i&3]
	}
}

// renderTestImage draws the probe pattern using only core/ widgets, so a
// successful frame also validates the shared toolkit over this transport.
// phase advances once per frame and drives the motion marker — a moving
// element is what distinguishes "our frames are holding the screen" from
// "one frame got stuck".
func renderTestImage(double, noXOR bool, phase int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, push3.VisW, push3.VisH))
	t := widgets.Default

	gfx.FillRect(img, 0, 0, push3.VisW, push3.VisH, t.Black)

	// Title bar.
	gfx.FillRect(img, 0, 0, push3.VisW, 22, t.CrumbBg)
	text.Draw(img, 8, 15, "push-tethered-app  /  frametest", t.CrumbCol)
	text.Draw(img, push3.VisW-190, 15, "Push 3 / controller mode", t.Gray)

	// Colour bars — primary channels first so a BGR/RGB swap is unmistakable.
	bars := []struct {
		label string
		c     color.NRGBA
	}{
		{"RED", color.NRGBA{255, 0, 0, 255}},
		{"GREEN", color.NRGBA{0, 255, 0, 255}},
		{"BLUE", color.NRGBA{0, 0, 255, 255}},
		{"WHITE", color.NRGBA{255, 255, 255, 255}},
		{"GRAY50", color.NRGBA{128, 128, 128, 255}},
		{"BLACK", color.NRGBA{0, 0, 0, 255}},
	}
	barW := push3.VisW / len(bars)
	for i, b := range bars {
		x := i * barW
		gfx.FillRect(img, x, 30, barW-2, 60, b.c)
		// Label below the bar, not on it, so it stays readable on black.
		text.Draw(img, x+6, 104, b.label, t.White)
	}

	// Motion marker: a block sweeping left to right, one lap every 2s at 30fps.
	markerW := 40
	mx := (phase * 8) % (push3.VisW - markerW)
	gfx.FillRect(img, mx, 112, markerW, 16, t.OnColor)
	text.Draw(img, 8, 124, fmt.Sprintf("frame %d", phase), t.Gray)

	// A one-pixel border proves we are addressing the full 960x160 extent and
	// that stride padding is not bleeding into the visible area.
	gfx.FillRect(img, 0, push3.VisH-1, push3.VisW, 1, t.OnColor)
	gfx.FillRect(img, 0, 0, 1, push3.VisH, t.OnColor)
	gfx.FillRect(img, push3.VisW-1, 0, 1, push3.VisH, t.OnColor)

	mode := "single frame"
	if double {
		mode = "double frame"
	}
	xorState := widgets.SoftOn
	xorLabel := "XOR ON"
	if noXOR {
		xorState = widgets.SoftOff
		xorLabel = "XOR OFF"
	}
	widgets.DrawBotStrip(img, t, push3.VisH-26, push3.VisW, push3.VisW/4, 24,
		[8]widgets.SoftButton{
			{Label: "IFACE 0", State: widgets.SoftOn},
			{Label: "EP 0x01", State: widgets.SoftOn},
			{Label: xorLabel, State: xorState},
			{Label: mode, State: widgets.SoftNeutral},
		}, "")

	return img
}
