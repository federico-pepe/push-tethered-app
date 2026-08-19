package display

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseSelector(t *testing.T) {
	tests := []struct {
		sel        string
		wantSerial string
		wantBus    int
		wantAddr   int
		wantErr    bool
	}{
		{sel: "serial:AB12CD34", wantSerial: "AB12CD34"},
		{sel: "usb:3.7", wantBus: 3, wantAddr: 7},
		{sel: "usb:0.0", wantBus: 0, wantAddr: 0},
		{sel: "serial:", wantErr: true},
		{sel: "usb:3", wantErr: true},
		{sel: "usb:x.7", wantErr: true},
		{sel: "usb:3.y", wantErr: true},
		{sel: "AB12CD34", wantErr: true},
		{sel: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.sel, func(t *testing.T) {
			serial, bus, addr, err := parseSelector(tt.sel)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSelector(%q) = (%q, %d, %d, nil), want error",
						tt.sel, serial, bus, addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSelector(%q): %v", tt.sel, err)
			}
			if serial != tt.wantSerial || bus != tt.wantBus || addr != tt.wantAddr {
				t.Errorf("parseSelector(%q) = (%q, %d, %d), want (%q, %d, %d)",
					tt.sel, serial, bus, addr, tt.wantSerial, tt.wantBus, tt.wantAddr)
			}
		})
	}
}

// A selector must survive the round trip through selectorFor, since a saved
// pairing is exactly that string written down and read back.
func TestSelectorRoundTrip(t *testing.T) {
	tests := []Info{
		{Model: "Push 3", Serial: "AB12CD34", Bus: 3, Address: 7},
		{Model: "Push 2", Bus: 1, Address: 12},
	}
	for _, want := range tests {
		sel := selectorFor(want)
		if !want.matches(sel) {
			t.Errorf("%v does not match its own selector %q", want, sel)
		}
	}
}

func TestInfoMatches(t *testing.T) {
	withSerial := Info{Model: "Push 3", Serial: "AB12CD34", Bus: 3, Address: 7}
	noSerial := Info{Model: "Push 3", Bus: 3, Address: 8}

	tests := []struct {
		name string
		info Info
		sel  string
		want bool
	}{
		{"serial matches", withSerial, "serial:AB12CD34", true},
		{"serial differs", withSerial, "serial:ZZ99ZZ99", false},
		// A serial selector must never fall back to bus/address: the unit at
		// that address after a replug may be a different box.
		{"serial selector ignores address", withSerial, "serial:", false},
		{"usb matches", withSerial, "usb:3.7", true},
		{"usb address differs", withSerial, "usb:3.8", false},
		{"usb bus differs", withSerial, "usb:4.7", false},
		{"usb matches unit without serial", noSerial, "usb:3.8", true},
		// The interesting case if Ableton ships no serial at all: a serial
		// selector must not match a unit that reports none, or every unit
		// would match every saved pairing.
		{"serial selector rejects unit without serial", noSerial, "serial:AB12CD34", false},
		{"malformed selector matches nothing", withSerial, "nonsense", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.matches(tt.sel); got != tt.want {
				t.Errorf("Info%+v.matches(%q) = %v, want %v", tt.info, tt.sel, got, tt.want)
			}
		})
	}
}

// Two units reporting the same serial is a real possibility — Ableton is not
// obliged to make it unique. Both then match one selector, which is why the UI
// has to be able to fall back to usb: form.
func TestDuplicateSerialsBothMatch(t *testing.T) {
	a := Info{Model: "Push 3", Serial: "SAME", Bus: 1, Address: 4}
	b := Info{Model: "Push 3", Serial: "SAME", Bus: 1, Address: 5}
	if !a.matches("serial:SAME") || !b.matches("serial:SAME") {
		t.Fatal("expected both units to match a shared serial")
	}
	if a.matches("usb:1.5") || !b.matches("usb:1.5") {
		t.Error("usb: form must still distinguish units with a shared serial")
	}
}

func TestSanitizeSerial(t *testing.T) {
	tests := []struct{ in, want string }{
		{"AB12CD34", "AB12CD34"},
		{"  AB12  ", "AB12"},
		// gousb substitutes "?" for non-ASCII in string descriptors; those
		// would render as missing-glyph boxes in core/gfx/text.
		{"AB?CD", "ABCD"},
		{"AB\x00\x1fCD", "ABCD"},
		{"AB\nCD", "ABCD"},
		{"", ""},
		{"???", ""},
	}
	for _, tt := range tests {
		if got := sanitizeSerial(tt.in); got != tt.want {
			t.Errorf("sanitizeSerial(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Ordering has to be stable across polls or the UI's unit rows reshuffle under
// the user mid-pairing. The per-product loop in enumerateUSB visits Push 3
// before Push 2, so without the sort a Push 2 on a lower bus would list last.
func TestSortUnitsByBusThenAddress(t *testing.T) {
	units := []Info{
		{Model: "Push 3", Bus: 3, Address: 2},
		{Model: "Push 3", Bus: 1, Address: 9},
		{Model: "Push 2", Bus: 1, Address: 4},
	}
	sortUnits(units)

	want := []string{"1.4", "1.9", "3.2"}
	for i, w := range want {
		got := fmt.Sprintf("%d.%d", units[i].Bus, units[i].Address)
		if got != w {
			t.Errorf("unit %d = bus/addr %s, want %s (full order: %v)", i, got, w, units)
		}
	}
}

func TestListUsesTheSeam(t *testing.T) {
	restore := enumerate
	defer func() { enumerate = restore }()
	want := []Info{{Model: "Push 3", Bus: 1, Address: 4, ID: "usb:1.4"}}
	enumerate = func() ([]Info, error) { return want, nil }

	got, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != want[0].ID {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

func TestListPropagatesError(t *testing.T) {
	restore := enumerate
	defer func() { enumerate = restore }()
	sentinel := errors.New("no permission")
	enumerate = func() ([]Info, error) { return nil, sentinel }

	if _, err := List(); !errors.Is(err, sentinel) {
		t.Errorf("List error = %v, want %v", err, sentinel)
	}
}

func TestInfoStringFlagsMissingSerial(t *testing.T) {
	withSerial := Info{Model: "Push 3", Serial: "AB12", Bus: 1, Address: 4, ID: "serial:AB12"}
	if s := withSerial.String(); strings.Contains(s, "no serial") {
		t.Errorf("String() = %q, should not mention a missing serial", s)
	}
	noSerial := Info{Model: "Push 2", Bus: 1, Address: 5, ID: "usb:1.5"}
	s := noSerial.String()
	if !strings.Contains(s, "no serial reported") {
		t.Errorf("String() = %q, want it to flag the missing serial", s)
	}
	if !strings.Contains(s, "usb:1.5") {
		t.Errorf("String() = %q, want it to carry the selector", s)
	}
}

func TestClaimRegistryRejectsDoubleClaim(t *testing.T) {
	const sel = "serial:TESTUNIT"
	if err := markClaimed(sel); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	defer releaseClaim(sel)

	err := markClaimed(sel)
	if !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("second claim = %v, want ErrAlreadyClaimed", err)
	}
	// The message must name the unit; "already claimed" alone is not
	// actionable once several units exist.
	if !strings.Contains(err.Error(), sel) {
		t.Errorf("error %q does not name the unit", err)
	}
}

func TestClaimRegistryReleases(t *testing.T) {
	const sel = "usb:9.9"
	if err := markClaimed(sel); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	releaseClaim(sel)
	if err := markClaimed(sel); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	releaseClaim(sel)
}

// Two different units must be claimable at once — that is the whole point.
func TestClaimRegistryAllowsDistinctUnits(t *testing.T) {
	a, b := "serial:UNIT-A", "serial:UNIT-B"
	if err := markClaimed(a); err != nil {
		t.Fatalf("claim %s: %v", a, err)
	}
	defer releaseClaim(a)
	if err := markClaimed(b); err != nil {
		t.Fatalf("claim %s while %s is held: %v", b, a, err)
	}
	releaseClaim(b)
}
