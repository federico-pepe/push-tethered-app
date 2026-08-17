package host

import (
	"errors"
	"testing"

	"github.com/federico-pepe/push-tethered-app/internal/midiout"
)

// These tests cover the lazy-open policy on the Runtime directly, without a MIDI
// port or a Push. They exist because the policy has already been wrong twice:
// first by opening the port unconditionally at startup, then by deciding from the
// set of *compiled-in* modules — which meant adding one module that sends MIDI
// made every run publish a system-wide port.
//
// On macOS and Linux, opening an output port advertises it to every other
// application on the machine. So "did we open it, and why" is user-visible
// behaviour, not an implementation detail.

func TestMIDIOutIsNotOpenedUntilAsked(t *testing.T) {
	calls := 0
	rt := &Runtime{opts: Options{
		OpenMIDIOut: func() (*midiout.Out, error) {
			calls++
			return nil, errors.New("should not have been called")
		},
	}}

	if calls != 0 {
		t.Fatalf("opener called %d times before any request", calls)
	}
	_, _ = rt.ensureMIDIOut()
	if calls != 1 {
		t.Errorf("opener called %d times after one request, want 1", calls)
	}
}

// TestMIDIOutFailureIsCachedNotRetried is the one with teeth. A module that
// sends on every pad press would otherwise retry a doomed open at input rate —
// and on Windows each attempt enumerates every MIDI port on the machine.
func TestMIDIOutFailureIsCachedNotRetried(t *testing.T) {
	calls := 0
	want := errors.New("no port")
	rt := &Runtime{opts: Options{
		OpenMIDIOut: func() (*midiout.Out, error) {
			calls++
			return nil, want
		},
	}}

	for i := 0; i < 50; i++ {
		out, err := rt.ensureMIDIOut()
		if out != nil {
			t.Fatal("got a port from a failing opener")
		}
		if !errors.Is(err, want) {
			t.Fatalf("attempt %d: err = %v, want %v", i, err, want)
		}
	}
	if calls != 1 {
		t.Errorf("opener called %d times across 50 requests, want 1", calls)
	}
}

func TestMIDIOutAbsentIsAClearError(t *testing.T) {
	rt := &Runtime{opts: Options{OpenMIDIOut: nil}}
	out, err := rt.ensureMIDIOut()
	if out != nil {
		t.Error("got a port with no opener configured")
	}
	if err == nil {
		t.Fatal("want an error when no opener is configured")
	}
}

// TestModuleHostSendsFailWithoutAPort covers the path a module actually takes:
// module.Host.SendCC/SendNote/NoteOff must report the failure rather than
// pretending to have sent something.
func TestModuleHostSendsFailWithoutAPort(t *testing.T) {
	rt := &Runtime{opts: Options{OpenMIDIOut: nil}}
	h := &moduleHost{rt: rt, id: "test"}

	if err := h.SendCC(1, 1, 64); err == nil {
		t.Error("SendCC returned nil with no port open")
	}
	if err := h.SendNote(1, 60, 100); err == nil {
		t.Error("SendNote returned nil with no port open")
	}
	if err := h.NoteOff(1, 60); err == nil {
		t.Error("NoteOff returned nil with no port open")
	}
}
