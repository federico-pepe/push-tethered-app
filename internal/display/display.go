// Package display owns Push's screen over USB: claiming the vendor-specific
// display interface and pushing frames to it.
//
// The pixel encoding itself is not reimplemented here — core/display.ToBGR565
// produces exactly the payload the bulk endpoint wants. This package adds the
// transport: device discovery, interface claiming, the frame header, the XOR
// line shaping, and the refresh loop.
//
// Protocol confirmed on tethered hardware 2026-08-09, see docs/archive/feasibility.md
// §8.3. It is identical on Push 2 and Push 3.
package display

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sort"
	"strings"
	"time"

	coredisplay "github.com/federico-pepe/ableton-push-hack/core/display"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/google/gousb"
)

const (
	// VendorAbleton is the USB vendor ID shared by Push 2 and Push 3.
	VendorAbleton = 0x2982
	// ProductPush2 / ProductPush3 are the product IDs.
	ProductPush2 = 0x1967
	ProductPush3 = 0x1969

	epDisplayOut = 1 // bulk OUT 0x01
	configNum    = 1 // the device has exactly one configuration

	classVendorSpecific = 0xFF
)

// ErrBusy reports that another process (in practice Ableton Live with Push as
// its control surface) holds the display interface. It is returned cleanly at
// claim time, before any write — callers should degrade rather than crash.
var ErrBusy = errors.New("display interface is claimed by another process (Live?)")

// ErrNotFound reports that no Push was found on USB.
var ErrNotFound = errors.New("no Push found on USB (connected and in controller mode?)")

// frameHeader precedes every frame: 0xFF 0xCC 0xAA 0x88 then 12 zero bytes.
var frameHeader = []byte{
	0xFF, 0xCC, 0xAA, 0x88,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

// xorPattern is the line shaping applied to pixel data (0xFFE7F3E7, phase-1).
var xorPattern = [4]byte{0xE7, 0xF3, 0xE7, 0xFF}

// Device is a claimed Push display.
type Device struct {
	ctx   *gousb.Context
	dev   *gousb.Device
	cfg   *gousb.Config
	intf  *gousb.Interface
	out   *gousb.OutEndpoint
	model string

	// buf is reused across frames so the refresh loop does not allocate
	// 320KB per frame at 30fps.
	buf []byte
}

// Model returns "Push 2" or "Push 3".
func (d *Device) Model() string { return d.model }

// Open finds a Push and claims ONLY the display interface. Audio (1-3) and
// MIDI (4-5) stay bound to the OS class drivers, which is what allows a DAW to
// keep using them — see docs/archive/feasibility.md §6.3.
func Open() (*Device, error) {
	ctx := gousb.NewContext()

	dev, err := ctx.OpenDeviceWithVIDPID(VendorAbleton, ProductPush3)
	model := "Push 3"
	if err == nil && dev == nil {
		dev, err = ctx.OpenDeviceWithVIDPID(VendorAbleton, ProductPush2)
		model = "Push 2"
	}
	if err != nil {
		ctx.Close()
		return nil, fmt.Errorf("opening Push: %w", err)
	}
	if dev == nil {
		ctx.Close()
		return nil, ErrNotFound
	}

	// NOTE: deliberately no dev.SetAutoDetach(true). It is config-wide, so it
	// would detach audio and MIDI from the OS class drivers too, destroying
	// co-existence mode — and on macOS it fails outright with
	// LIBUSB_ERROR_ACCESS. The display interface is vendor-specific with no
	// class driver bound, so there is nothing to detach anyway.

	cfg, err := dev.Config(configNum)
	if err != nil {
		dev.Close()
		ctx.Close()
		return nil, fmt.Errorf("selecting configuration %d: %w", configNum, err)
	}

	ifaceNum, err := findDisplayInterface(dev, cfg)
	if err != nil {
		cfg.Close()
		dev.Close()
		ctx.Close()
		return nil, err
	}

	intf, err := cfg.Interface(ifaceNum, 0)
	if err != nil {
		cfg.Close()
		dev.Close()
		ctx.Close()
		if isAccessError(err) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("claiming interface %d: %w", ifaceNum, err)
	}

	out, err := intf.OutEndpoint(epDisplayOut)
	if err != nil {
		intf.Close()
		cfg.Close()
		dev.Close()
		ctx.Close()
		return nil, fmt.Errorf("opening OUT endpoint %#02x: %w", epDisplayOut, err)
	}

	return &Device{
		ctx: ctx, dev: dev, cfg: cfg, intf: intf, out: out,
		model: model,
		buf:   make([]byte, push3.FrameBytes),
	}, nil
}

// findDisplayInterface picks the vendor-specific interface that drives the
// screen, rather than assuming interface 0.
//
// This matters for two reasons. Push 2 and Push 3 are different devices with
// different interface layouts — Push 3 is a composite device with audio and
// MIDI, Push 2 is not — so a hardcoded number is a Push-3 assumption.
//
// More importantly, Push 3 has *two* vendor-specific interfaces: 0 ("Display")
// and 6 ("xPort"), and their descriptors are identical — both 255/255/255 with
// two bulk endpoints. Nothing but the interface string tells them apart.
// xPort is undocumented and CLAUDE.md forbids writing to it, so this selects
// by name and refuses to fall back onto it.
func findDisplayInterface(dev *gousb.Device, cfg *gousb.Config) (int, error) {
	var vendorIfaces []int
	for _, iface := range cfg.Desc.Interfaces {
		for _, alt := range iface.AltSettings {
			if alt.Class != classVendorSpecific {
				continue
			}
			name, err := dev.InterfaceDescription(cfg.Desc.Number, iface.Number, alt.Alternate)
			if err == nil {
				if strings.Contains(strings.ToLower(name), "display") {
					return iface.Number, nil
				}
				if strings.Contains(strings.ToLower(name), "xport") {
					continue // never claim the undocumented interface
				}
			}
			vendorIfaces = append(vendorIfaces, iface.Number)
			break
		}
	}

	// No interface advertised itself as the display. Fall back to the lowest
	// vendor-specific interface, which is the display on both known devices —
	// but only if there is exactly one candidate, so we can never guess xPort.
	switch len(vendorIfaces) {
	case 0:
		return 0, errors.New("no vendor-specific display interface found")
	case 1:
		return vendorIfaces[0], nil
	default:
		sort.Ints(vendorIfaces)
		return 0, fmt.Errorf("cannot identify the display interface: %d vendor-specific "+
			"candidates %v and none is named \"Display\" — refusing to guess", len(vendorIfaces), vendorIfaces)
	}
}

// isAccessError reports whether err is libusb's LIBUSB_ERROR_ACCESS, which is
// how "someone else owns this interface" surfaces.
func isAccessError(err error) bool {
	var se gousb.TransferStatus
	if errors.As(err, &se) {
		return false
	}
	// gousb wraps the libusb error in a plain error for claim failures, so the
	// string check is a necessary fallback alongside the typed comparison.
	msg := err.Error()
	return errors.Is(err, gousb.ErrorAccess) ||
		strings.Contains(msg, "bad access") ||
		strings.Contains(msg, "LIBUSB_ERROR_ACCESS")
}

// WriteFrame encodes img and pushes one frame. A single frame is sufficient —
// the standalone device's frame duplication is a quirk of Ableton's binary,
// not a hardware requirement (docs/archive/feasibility.md §8.3).
func (d *Device) WriteFrame(ctx context.Context, img image.Image) error {
	full := coredisplay.ToBGR565(img)
	copy(d.buf, full[:push3.FrameBytes])
	for i := range d.buf {
		d.buf[i] ^= xorPattern[i&3]
	}

	wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := d.out.WriteContext(wctx, frameHeader); err != nil {
		return fmt.Errorf("writing frame header: %w", err)
	}
	if _, err := d.out.WriteContext(wctx, d.buf); err != nil {
		return fmt.Errorf("writing pixels: %w", err)
	}
	return nil
}

// Blank pushes an all-black frame. Worth calling on shutdown: Push redraws its
// own idle screen shortly after we stop, but a black frame makes the handover
// clean rather than leaving our last frame frozen for a moment.
func (d *Device) Blank(ctx context.Context) error {
	return d.WriteFrame(ctx, image.NewNRGBA(image.Rect(0, 0, push3.VisW, push3.VisH)))
}

// Close releases the interface and the device, in reverse order of claiming.
func (d *Device) Close() error {
	if d.intf != nil {
		d.intf.Close()
	}
	if d.cfg != nil {
		d.cfg.Close()
	}
	if d.dev != nil {
		d.dev.Close()
	}
	if d.ctx != nil {
		return d.ctx.Close()
	}
	return nil
}
