// Package midiout owns a MIDI output port that other software on the machine
// can receive from. It is how a module reaches a synth, a DAW, or anything else
// that speaks MIDI.
//
// The key design point: this package does NOT "create a virtual port". It
// **owns a named output port**, obtained one of two ways, because virtual-port
// creation is not portable. Measured in the vendored RtMidi sources at
// gitlab.com/gomidi/midi/v2@v2.3.24:
//
//   - macOS  — MidiOutCore::openVirtualPort calls MIDISourceCreate, so other
//     apps see a new MIDI input they can subscribe to (RtMidi.cpp:1637).
//   - Linux  — MidiOutAlsa::openVirtualPort creates an ALSA seq port
//     (RtMidi.cpp:2553).
//   - Windows — MidiOutWinMM::openVirtualPort refuses outright:
//     "MidiOutWinMM::openVirtualPort: cannot be implemented in Windows MM MIDI
//     API!" (RtMidi.cpp:3128). WinUWP is the same (:3947). It is a *warning*;
//     no port is created.
//
// So Open tries to create first and falls back to attaching to an existing
// port by name. On Windows the user supplies that port with loopMIDI (free) or
// Windows MIDI Services. Mode reports which path was taken. No build tags, and
// if Windows ever gains native virtual ports the create path starts working
// with no change here.
//
// Feedback-loop guard: attaching never considers a port whose name mentions
// Push. Sending our output into Push's own input would loop straight back
// through internal/midi's decoder.
package midiout

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/federico-pepe/push-tethered-app/internal/midilock"
	gm "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // RtMidi C++ is vendored; no system package needed
)

// DefaultName is the port name used when the caller doesn't pick one. On macOS
// and Linux this is the name other apps will see; ASCII only, since it reaches
// CoreMIDI as kCFStringEncodingASCII.
const DefaultName = "Push Tethered App"

// Mode records how the port was obtained. Worth surfacing to the user: the two
// modes have very different setup stories, and "attached" means someone else
// owns the port's lifetime.
type Mode string

const (
	// ModeVirtual means we created the port ourselves (macOS, Linux).
	ModeVirtual Mode = "virtual"
	// ModeAttached means we opened a port that already existed (Windows).
	ModeAttached Mode = "attached"
)

// ErrNoPort reports that neither path worked: this platform can't create a
// virtual port and no existing port matched. On Windows this is the expected
// first-run error, and the fix is a loopback driver, not a code change.
var ErrNoPort = errors.New("no MIDI output port available")

// virtualOpener is the capability we need from the registered driver.
// rtmididrv satisfies it (drivers/rtmididrv/driver.go:105); asserting on the
// behaviour rather than the concrete type keeps this package driver-agnostic
// and makes a driver without virtual-port support fall back cleanly.
type virtualOpener interface {
	OpenVirtualOut(name string) (drivers.Out, error)
}

// Out is an owned MIDI output port.
//
// Safe for concurrent use. The host serialises module calls onto one goroutine
// anyway, but a module is free to keep its own timer goroutine and a mutex here
// is far cheaper than a data race in somebody else's sequencer.
type Out struct {
	mu   sync.Mutex
	out  drivers.Out
	name string
	mode Mode
}

// Open obtains an output port. An empty name means DefaultName when creating;
// when attaching it means "the first port that isn't Push", which is a
// convenience for a single-loopback-port machine rather than something to rely
// on.
func Open(name string) (*Out, error) {
	if name == "" {
		name = DefaultName
	}

	// rtmididrv's opened-ports list has no locking of its own — see
	// internal/midilock — so every entry into the driver from this process
	// (here and in internal/midi) has to go through this lock.
	midilock.Lock()
	defer midilock.Unlock()

	if drv, ok := drivers.Get().(virtualOpener); ok {
		if port, err := drv.OpenVirtualOut(name); err == nil {
			return &Out{out: port, name: name, mode: ModeVirtual}, nil
		}
		// Fall through: on Windows this always fails, and that is not fatal.
	}

	port, actual, err := attach(name)
	if err != nil {
		return nil, err
	}
	return &Out{out: port, name: actual, mode: ModeAttached}, nil
}

// attach finds an existing output port matching name, case-insensitively by
// substring, skipping anything that looks like Push itself.
func attach(name string) (drivers.Out, string, error) {
	want := strings.ToLower(name)

	var candidates []string
	for _, p := range gm.GetOutPorts() {
		n := p.String()
		if isPush(n) {
			continue
		}
		candidates = append(candidates, n)
		if want == "" || strings.Contains(strings.ToLower(n), want) {
			if err := p.Open(); err != nil {
				return nil, "", fmt.Errorf("opening MIDI out %q: %w", n, err)
			}
			return p, n, nil
		}
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("%w: this platform cannot create one and no "+
			"other MIDI output exists (on Windows, install loopMIDI and create a port)",
			ErrNoPort)
	}
	return nil, "", fmt.Errorf("%w: no output port matching %q; available: %v",
		ErrNoPort, name, candidates)
}

// isPush reports whether a port name looks like the Push hardware. Used to keep
// attach from wiring our output back into Push's own input.
func isPush(portName string) bool {
	return strings.Contains(strings.ToLower(portName), "push")
}

// Name returns the port name as it appears to other software.
func (o *Out) Name() string { return o.name }

// Mode reports whether the port was created or attached to.
func (o *Out) Mode() Mode { return o.mode }

// SendCC sends a control change. ch is 1-16, matching how channels are
// described everywhere else in this project (channel 1 is Push's control
// surface); it is converted to the wire's 0-15 here so no caller has to
// remember which convention it is holding.
func (o *Out) SendCC(ch, cc, val byte) error {
	status, err := status(0xB0, ch)
	if err != nil {
		return err
	}
	return o.send(status, cc&0x7F, val&0x7F)
}

// SendNote sends a note on. A velocity of 0 is a note off, per the MIDI spec's
// running-status convention — the same rule remap.go relies on upstream.
func (o *Out) SendNote(ch, note, vel byte) error {
	status, err := status(0x90, ch)
	if err != nil {
		return err
	}
	return o.send(status, note&0x7F, vel&0x7F)
}

// NoteOff sends an explicit note off.
func (o *Out) NoteOff(ch, note byte) error {
	status, err := status(0x80, ch)
	if err != nil {
		return err
	}
	return o.send(status, note&0x7F, 0)
}

// SendClock sends one MIDI timing clock tick (0xF8) — a system realtime
// message, no channel and no data bytes. Send 24 per quarter note to drive
// another device's tempo; see SendStart/SendContinue/SendStop for the
// transport messages that go with it.
func (o *Out) SendClock() error { return o.send(0xF8) }

// SendStart sends MIDI Start (0xFA): begin playback from the top.
func (o *Out) SendStart() error { return o.send(0xFA) }

// SendContinue sends MIDI Continue (0xFB): resume playback from wherever it
// was stopped, as opposed to Start's "from the top".
func (o *Out) SendContinue() error { return o.send(0xFB) }

// SendStop sends MIDI Stop (0xFC).
func (o *Out) SendStop() error { return o.send(0xFC) }

// status builds a status byte from a message kind and a 1-16 channel.
func status(kind, ch byte) (byte, error) {
	if ch < 1 || ch > 16 {
		return 0, fmt.Errorf("MIDI channel %d out of range (want 1-16)", ch)
	}
	return kind | (ch - 1), nil
}

func (o *Out) send(b ...byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.out == nil {
		return drivers.ErrPortClosed
	}
	return o.out.Send(b)
}

// Close releases the port. Safe to call twice.
func (o *Out) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.out != nil && o.out.IsOpen() {
		_ = o.out.Close()
	}
	o.out = nil
}
