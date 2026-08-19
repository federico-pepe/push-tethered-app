package display

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/gousb"
)

// vendorIface builds an interface descriptor with one vendor-specific alt
// setting, which is the shape both Push 3's Display and its xPort present.
func vendorIface(num int) gousb.InterfaceDesc {
	return gousb.InterfaceDesc{
		Number: num,
		AltSettings: []gousb.InterfaceSetting{
			{Number: num, Alternate: 0, Class: classVendorSpecific},
		},
	}
}

func classIface(num int, class gousb.Class) gousb.InterfaceDesc {
	return gousb.InterfaceDesc{
		Number: num,
		AltSettings: []gousb.InterfaceSetting{
			{Number: num, Alternate: 0, Class: class},
		},
	}
}

// named returns a describe func backed by a fixed interface-number → string
// map. A missing entry reports an error, which is how a device that refuses to
// hand over string descriptors behaves.
func named(names map[int]string) func(iface, alt int) (string, error) {
	return func(iface, alt int) (string, error) {
		if s, ok := names[iface]; ok {
			return s, nil
		}
		return "", errors.New("no string descriptor")
	}
}

func TestPickDisplayInterfaceByName(t *testing.T) {
	// Push 3: interface 0 is Display, 6 is the undocumented xPort, and their
	// descriptors are identical apart from the string.
	ifaces := []gousb.InterfaceDesc{
		vendorIface(0),
		classIface(1, gousb.ClassAudio),
		classIface(5, gousb.ClassAudio),
		vendorIface(6),
	}
	got, err := pickDisplayInterface(ifaces, named(map[int]string{
		0: "Ableton Push 3 Display",
		6: "Ableton Push 3 xPort",
	}))
	if err != nil {
		t.Fatalf("pickDisplayInterface: %v", err)
	}
	if got != 0 {
		t.Errorf("picked interface %d, want 0 (Display)", got)
	}
}

// The name match must win regardless of descriptor order, so that a device
// listing xPort first cannot shift the outcome.
func TestPickDisplayInterfaceIgnoresOrder(t *testing.T) {
	ifaces := []gousb.InterfaceDesc{vendorIface(6), vendorIface(0)}
	got, err := pickDisplayInterface(ifaces, named(map[int]string{
		0: "Display",
		6: "xPort",
	}))
	if err != nil {
		t.Fatalf("pickDisplayInterface: %v", err)
	}
	if got != 0 {
		t.Errorf("picked interface %d, want 0", got)
	}
}

// Push 2 has a single vendor-specific interface and no useful string, so the
// lone-candidate fallback has to keep working.
func TestPickDisplayInterfaceSingleUnnamedCandidate(t *testing.T) {
	ifaces := []gousb.InterfaceDesc{vendorIface(0), classIface(1, gousb.ClassAudio)}
	got, err := pickDisplayInterface(ifaces, named(nil))
	if err != nil {
		t.Fatalf("pickDisplayInterface: %v", err)
	}
	if got != 0 {
		t.Errorf("picked interface %d, want 0", got)
	}
}

// The branch that guards against ever claiming xPort: two indistinguishable
// vendor-specific interfaces and no name to tell them apart must refuse rather
// than take the lower number. CLAUDE.md forbids writing to xPort, and a wrong
// guess here writes to an undocumented vendor interface.
func TestPickDisplayInterfaceRefusesToGuess(t *testing.T) {
	ifaces := []gousb.InterfaceDesc{vendorIface(0), vendorIface(6)}
	got, err := pickDisplayInterface(ifaces, named(nil))
	if err == nil {
		t.Fatalf("picked interface %d, want a refusal", got)
	}
	for _, want := range []string{"refusing to guess", "[0 6]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// An xPort we *can* name is skipped, not counted as a candidate — otherwise a
// Push 3 whose Display string is unreadable would trip the refusal above
// instead of falling back to the one remaining interface.
func TestPickDisplayInterfaceSkipsNamedXPort(t *testing.T) {
	ifaces := []gousb.InterfaceDesc{vendorIface(0), vendorIface(6)}
	got, err := pickDisplayInterface(ifaces, named(map[int]string{6: "xPort"}))
	if err != nil {
		t.Fatalf("pickDisplayInterface: %v", err)
	}
	if got != 0 {
		t.Errorf("picked interface %d, want 0", got)
	}
}

func TestPickDisplayInterfaceNoVendorInterface(t *testing.T) {
	ifaces := []gousb.InterfaceDesc{classIface(0, gousb.ClassAudio)}
	if got, err := pickDisplayInterface(ifaces, named(nil)); err == nil {
		t.Fatalf("picked interface %d, want an error", got)
	}
}

// SetAutoDetach(true) is config-wide, not interface-wide: it would tear Push's
// audio and MIDI interfaces away from the OS class drivers, and on macOS it
// fails outright with LIBUSB_ERROR_ACCESS. CLAUDE.md and
// docs/protocol/usb-and-safety.md both forbid it.
//
// Reading our own source in a test is unusual, but a silent reintroduction here
// costs the user their audio and MIDI, and the call is easy to add back during
// a refactor without anyone noticing in review.
func TestNoSetAutoDetach(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("globbing package sources: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no package sources found — the guard would silently pass")
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		// The forbidden call, spelled so this line does not match itself. Only
		// non-comment lines count — the package deliberately documents the
		// rule with a comment naming the call it must never make.
		forbidden := "SetAutoDetach(" + "true)"
		for _, line := range strings.Split(string(src), "\n") {
			if strings.Contains(strings.TrimSpace(line), "//") {
				continue
			}
			if strings.Contains(line, forbidden) {
				t.Errorf("%s calls SetAutoDetach(true) — see docs/protocol/usb-and-safety.md", f)
			}
		}
	}
}
