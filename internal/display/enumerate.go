package display

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/gousb"
)

// ErrAlreadyClaimed reports that this process already drives the requested
// unit's display. It is deliberately distinct from ErrBusy: ErrBusy means
// somebody else (in practice Live) owns the interface and the caller should
// degrade, whereas this means our own caller asked twice for the same screen,
// which is a bug in the caller and not a condition to work around.
var ErrAlreadyClaimed = errors.New("this Push's display is already claimed by this process")

// Info identifies one Push on USB without claiming anything on it.
//
// Two selector forms exist because neither works alone. Serial survives
// replugging and an application restart, which is what a saved pairing needs,
// but reading it takes an open device handle and Ableton is not obliged to make
// it unique — or to report one at all. Bus and address need no handle but are
// reassigned on replug, and gousb does not expose libusb's full port-number
// path, so there is no stable topology identifier available either.
type Info struct {
	Model   string `json:"model"`  // "Push 2" or "Push 3"
	Serial  string `json:"serial"` // ASCII; "" when the device reports none
	Bus     int    `json:"bus"`
	Address int    `json:"address"`
	Port    int    `json:"port"` // hub port; informational, not a stable path

	// ID is the selector that addresses this unit — "serial:..." when the
	// device reports one, else "usb:BUS.ADDR". Pass it to OpenID.
	ID string `json:"id"`
}

// String renders an Info for logs and for cmd/pushapp -devices.
func (i Info) String() string {
	s := fmt.Sprintf("%s [%s] bus %d addr %d", i.Model, i.ID, i.Bus, i.Address)
	if i.Serial == "" {
		s += " (no serial reported)"
	}
	return s
}

// productModels maps the product IDs we know to their marketing names. Push 3
// is listed first so that Open keeps preferring it, as it always has.
var productModels = []struct {
	product int
	model   string
}{
	{ProductPush3, "Push 3"},
	{ProductPush2, "Push 2"},
}

// enumerate is the seam the tests replace. The real implementation walks USB;
// gousb's fake libusb backend is unexported, so a package-level function
// variable is the only way to exercise List's callers without hardware.
var enumerate = enumerateUSB

// List returns every Push currently on USB, ordered by bus then address so the
// ordering is stable across polls.
//
// It opens a device handle per unit to read the serial string descriptor, then
// closes it again. Opening is not claiming: no configuration is selected and no
// interface is claimed, so List stays safe to call while Live owns the display
// and while another session in this process is driving the unit.
func List() ([]Info, error) { return enumerate() }

func enumerateUSB() ([]Info, error) {
	ctx := acquireCtx()
	defer releaseCtx()

	var found []Info
	for _, pm := range productModels {
		devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
			return desc.Vendor == gousb.ID(VendorAbleton) && desc.Product == gousb.ID(pm.product)
		})
		// OpenDevices returns the devices it managed to open alongside any
		// error, so close what we got either way. A per-device permission
		// failure (a missing udev rule on Linux, say) must not lose the units
		// we could read.
		for _, dev := range devs {
			info := Info{
				Model:   pm.model,
				Bus:     dev.Desc.Bus,
				Address: dev.Desc.Address,
				Port:    dev.Desc.Port,
			}
			// A serial we cannot read is not fatal — the usb: selector still
			// addresses the unit.
			if s, err := dev.SerialNumber(); err == nil {
				info.Serial = sanitizeSerial(s)
			}
			info.ID = selectorFor(info)
			found = append(found, info)
			dev.Close()
		}
		if err != nil && len(devs) == 0 {
			return nil, fmt.Errorf("enumerating %s: %w", pm.model, err)
		}
	}

	sortUnits(found)
	return found, nil
}

// sortUnits orders units by bus then address, so that repeated List calls
// present the same order to the UI even though libusb's enumeration order and
// our per-product loop do not guarantee one.
func sortUnits(units []Info) {
	sort.Slice(units, func(a, b int) bool {
		if units[a].Bus != units[b].Bus {
			return units[a].Bus < units[b].Bus
		}
		return units[a].Address < units[b].Address
	})
}

// openMatch opens the unit sel addresses and returns the still-open handle
// along with its Info. An empty sel keeps Open's historical behaviour: the
// first unit found, Push 3 preferred over Push 2.
//
// Every candidate has to be opened to be identified, because the serial lives
// in a string descriptor and gousb's matcher only sees the device descriptor.
// Candidates that turn out not to match are closed again before returning.
//
// The caller owns the returned handle and must releaseCtx once it is closed.
func openMatch(sel string) (*gousb.Device, Info, error) {
	ctx := acquireCtx()

	var (
		match     *gousb.Device
		matchInfo Info
		seen      []Info
		firstErr  error
	)
	for _, pm := range productModels {
		devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
			return desc.Vendor == gousb.ID(VendorAbleton) && desc.Product == gousb.ID(pm.product)
		})
		if err != nil && len(devs) == 0 && firstErr == nil {
			firstErr = err
		}
		for _, dev := range devs {
			info := Info{
				Model:   pm.model,
				Bus:     dev.Desc.Bus,
				Address: dev.Desc.Address,
				Port:    dev.Desc.Port,
			}
			if s, err := dev.SerialNumber(); err == nil {
				info.Serial = sanitizeSerial(s)
			}
			info.ID = selectorFor(info)
			seen = append(seen, info)

			wanted := sel == "" || info.matches(sel)
			if wanted && match == nil {
				match, matchInfo = dev, info
				continue
			}
			dev.Close()
		}
		// Push 3 wins over Push 2 when no selector was given, so stop as soon
		// as the higher-priority product produced a match.
		if match != nil && sel == "" {
			break
		}
	}

	if match != nil {
		return match, matchInfo, nil
	}
	releaseCtx()

	switch {
	case sel == "" && firstErr != nil:
		return nil, Info{}, fmt.Errorf("opening Push: %w", firstErr)
	case sel == "":
		return nil, Info{}, ErrNotFound
	case len(seen) == 0:
		return nil, Info{}, fmt.Errorf("%w: no unit matches %q and none is connected", ErrNotFound, sel)
	default:
		return nil, Info{}, fmt.Errorf("%w: no unit matches %q; connected: %s",
			ErrNotFound, sel, describe(seen))
	}
}

// describe renders a unit list for an error message, so a failed selector says
// what the user could have asked for instead.
func describe(units []Info) string {
	ids := make([]string, len(units))
	for i, u := range units {
		ids[i] = u.String()
	}
	return strings.Join(ids, "; ")
}

// sanitizeSerial strips anything that would render as a missing-glyph box in
// core/gfx/text, which is ASCII-only. gousb already substitutes "?" for
// characters outside ASCII when it decodes a string descriptor, so those get
// dropped here too rather than reaching the screen.
func sanitizeSerial(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7F && r != '?' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// selectorFor picks the selector for a unit: serial when it reports one,
// otherwise its current bus and address.
func selectorFor(i Info) string {
	if i.Serial != "" {
		return "serial:" + i.Serial
	}
	return fmt.Sprintf("usb:%d.%d", i.Bus, i.Address)
}

// matches reports whether sel addresses this unit. The comparison is exact in
// both forms — a near-miss here would drive the wrong physical screen, so
// there is deliberately no fuzzy or prefix matching.
func (i Info) matches(sel string) bool {
	serial, bus, addr, err := parseSelector(sel)
	if err != nil {
		return false
	}
	if serial != "" {
		return i.Serial != "" && i.Serial == serial
	}
	return i.Bus == bus && i.Address == addr
}

// parseSelector splits a selector into its serial form or its bus/address
// form. Exactly one of serial and (bus, addr) is meaningful in the result.
func parseSelector(sel string) (serial string, bus, addr int, err error) {
	switch {
	case strings.HasPrefix(sel, "serial:"):
		serial = strings.TrimPrefix(sel, "serial:")
		if serial == "" {
			return "", 0, 0, fmt.Errorf("selector %q: empty serial", sel)
		}
		return serial, 0, 0, nil

	case strings.HasPrefix(sel, "usb:"):
		rest := strings.TrimPrefix(sel, "usb:")
		b, a, ok := strings.Cut(rest, ".")
		if !ok {
			return "", 0, 0, fmt.Errorf("selector %q: want usb:BUS.ADDR", sel)
		}
		bus, err = strconv.Atoi(b)
		if err != nil {
			return "", 0, 0, fmt.Errorf("selector %q: bad bus %q", sel, b)
		}
		addr, err = strconv.Atoi(a)
		if err != nil {
			return "", 0, 0, fmt.Errorf("selector %q: bad address %q", sel, a)
		}
		return "", bus, addr, nil

	default:
		return "", 0, 0, fmt.Errorf("selector %q: want serial:... or usb:BUS.ADDR", sel)
	}
}

// The libusb context is shared process-wide and reference counted.
//
// gousb's Context.Close errors while any device opened from it is still open,
// so handing every Device its own context invites a close-ordering bug that
// only shows up once a second unit exists. One context also means one libusb
// event-handling thread rather than N. Every entry point that opens a device
// acquires, and Device.Close releases after the device itself is closed.
var (
	ctxMu   sync.Mutex
	usbCtx  *gousb.Context
	ctxRefs int
)

func acquireCtx() *gousb.Context {
	ctxMu.Lock()
	defer ctxMu.Unlock()
	if usbCtx == nil {
		usbCtx = gousb.NewContext()
	}
	ctxRefs++
	return usbCtx
}

func releaseCtx() {
	ctxMu.Lock()
	defer ctxMu.Unlock()
	ctxRefs--
	if ctxRefs > 0 || usbCtx == nil {
		return
	}
	// Nothing holds a device any more, so closing cannot fail on an open
	// handle. The error is dropped deliberately: releaseCtx runs on cleanup
	// paths that have nowhere to report it, and a failure to close a libusb
	// context has no recovery.
	_ = usbCtx.Close()
	usbCtx = nil
}

// claimed tracks which units this process is already driving, keyed by the
// selector OpenID resolved. Without it a second OpenID for the same unit
// surfaces as ErrBusy — whose message blames Live — for what is really our own
// double-claim.
var (
	claimedMu sync.Mutex
	claimed   = map[string]bool{}
)

func markClaimed(sel string) error {
	claimedMu.Lock()
	defer claimedMu.Unlock()
	if claimed[sel] {
		return fmt.Errorf("%w: %s", ErrAlreadyClaimed, sel)
	}
	claimed[sel] = true
	return nil
}

func releaseClaim(sel string) {
	claimedMu.Lock()
	defer claimedMu.Unlock()
	delete(claimed, sel)
}
