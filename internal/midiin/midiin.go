// Package midiin owns a MIDI input port that other software on the machine
// can send into — the mirror image of internal/midiout. It exists so a
// module can be driven by something external: an incoming MIDI clock to sync
// a sequencer to, or (nothing stops it — the wire carries raw bytes) notes
// and CCs from a controller.
//
// Same portability story as internal/midiout, same vendored driver
// (gitlab.com/gomidi/midi/v2/drivers/rtmididrv), same asymmetry: macOS and
// Linux can create a virtual port (rtmididrv's OpenVirtualIn, mirroring
// OpenVirtualOut); Windows cannot, so Open falls back to attaching to an
// existing port by name, same loopMIDI story as the output side.
//
// Deliberately does no decoding. A module receives raw bytes
// (module.ExternalMIDI) and decides what they mean — this package's job
// stops at "these bytes arrived", the same way internal/midiout's job stops
// at "these bytes were sent". Keeping decode policy out of here means a
// clock byte, a note, and a CC all reach a module the same way, with no
// assumption baked in about what any particular module wants to listen for.
package midiin

import (
	"fmt"
	"strings"

	"github.com/federico-pepe/push-tethered-app/internal/midilock"
	gm "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // vendored; see internal/midiout
)

// DefaultName is the port name used when the caller doesn't pick one.
// Deliberately distinct from midiout.DefaultName — the two are separate
// unidirectional virtual ports, and giving them different names avoids
// showing up as one ambiguous entry in another app's port list.
const DefaultName = "Push Tethered App In"

// ErrNoPort mirrors midiout.ErrNoPort: this platform can't create a virtual
// input and no existing port matched.
var ErrNoPort = fmt.Errorf("no MIDI input port available")

type virtualInOpener interface {
	OpenVirtualIn(name string) (drivers.In, error)
}

// In is an owned MIDI input port. Not safe for concurrent Listen calls —
// callers are expected to call it once, same as internal/midi.Port.Listen.
type In struct {
	in   drivers.In
	name string
	stop func()
}

// Open obtains an input port. An empty name means DefaultName when creating;
// when attaching it means "the first non-Push port", same convenience and
// same caveat as midiout.Open.
func Open(name string) (*In, error) {
	if name == "" {
		name = DefaultName
	}

	midilock.Lock()
	defer midilock.Unlock()

	if drv, ok := drivers.Get().(virtualInOpener); ok {
		if port, err := drv.OpenVirtualIn(name); err == nil {
			return &In{in: port, name: name}, nil
		}
		// Fall through: on Windows this always fails, and that is not fatal.
	}

	port, actual, err := attach(name)
	if err != nil {
		return nil, err
	}
	return &In{in: port, name: actual}, nil
}

// OpenExisting attaches to a specific input port already listed by the
// driver, by exact name and driver-assigned number. Unlike Open/attach it
// does not skip Push-named ports — this is how a caller reaches Push 3's own
// External Port cable (the physical DIN jack), which looks like a Push port
// by name but is not the control-surface port internal/midi already owns.
//
// Re-checks that num still reports name before opening, the same
// port-list-changed guard internal/midi.OpenRef uses, since a stale ref
// (unit unplugged since the caller enumerated it) must not silently open
// whatever now sits at that driver slot.
func OpenExisting(name string, num int) (*In, error) {
	midilock.Lock()
	defer midilock.Unlock()

	for _, p := range gm.GetInPorts() {
		if p.Number() != num {
			continue
		}
		if p.String() != name {
			return nil, fmt.Errorf("MIDI port list changed: port %d is now %q, expected %q — re-enumerate and retry",
				num, p.String(), name)
		}
		if err := p.Open(); err != nil {
			return nil, fmt.Errorf("opening MIDI in %q: %w", name, err)
		}
		return &In{in: p, name: name}, nil
	}
	return nil, fmt.Errorf("no MIDI input port %d (%q) found", num, name)
}

// attach finds an existing input port matching name, case-insensitively by
// substring, skipping anything that looks like Push itself — this is a
// receive port for *other* software's MIDI, not a second way to read Push's
// own control surface (internal/midi already owns that).
func attach(name string) (drivers.In, string, error) {
	want := strings.ToLower(name)

	var candidates []string
	for _, p := range gm.GetInPorts() {
		n := p.String()
		if isPush(n) {
			continue
		}
		candidates = append(candidates, n)
		if want == "" || strings.Contains(strings.ToLower(n), want) {
			if err := p.Open(); err != nil {
				return nil, "", fmt.Errorf("opening MIDI in %q: %w", n, err)
			}
			return p, n, nil
		}
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("%w: this platform cannot create one and no "+
			"other MIDI input exists (on Windows, install loopMIDI and create a port)",
			ErrNoPort)
	}
	return nil, "", fmt.Errorf("%w: no input port matching %q; available: %v",
		ErrNoPort, name, candidates)
}

func isPush(portName string) bool {
	return strings.Contains(strings.ToLower(portName), "push")
}

// Name returns the port name as it appears to other software.
func (i *In) Name() string { return i.name }

// Listen delivers every incoming message's raw bytes to fn until Close.
//
// drivers.ListenConfig.TimeCode is forced on (not the zero value) —
// confirmed live 2026-08-20: RtMidi's underlying ignoreTypes has a
// "midiTime" flag that, despite the name, silently drops MIDI Clock
// (0xF8) at the C library level, before this package's decoder ever sees
// it, and gomidi's TimeCode config bit is what turns that flag off. Without
// this, Start/Stop/Continue (a different RtMidi message class) arrive fine
// and Clock never does — exactly the "receives the first beat, then
// nothing" symptom that surfaced this. Active Sensing and System Exclusive
// are still filtered (ListenConfig's other defaults); everything else,
// including Clock/Start/Stop/Continue and ordinary channel messages, passes
// through unfiltered and undecoded.
func (i *In) Listen(fn func(raw []byte)) error {
	stop, err := i.in.Listen(func(msg []byte, _ int32) {
		fn(msg)
	}, drivers.ListenConfig{TimeCode: true})
	if err != nil {
		return fmt.Errorf("listening on %q: %w", i.name, err)
	}
	i.stop = stop
	return nil
}

// Close releases the port. Safe to call twice.
func (i *In) Close() {
	if i.stop != nil {
		i.stop()
		i.stop = nil
	}
	if i.in != nil && i.in.IsOpen() {
		_ = i.in.Close()
	}
}
