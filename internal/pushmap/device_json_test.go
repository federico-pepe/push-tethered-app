package pushmap

import (
	"encoding/json"
	"testing"
)

// Device crosses into JSON as part of midi.PortRef, sent to pushapp-ui's
// frontend by Overview and sent back unchanged by Connect — a real pairing
// attempt failed with "cannot unmarshal string into Go struct field ...
// device of type pushmap.Device" until UnmarshalJSON was added, because
// MarshalJSON encodes as a string but the default unmarshaller expects the
// underlying int. This is that round trip, asserted directly rather than
// only through the UI that happened to catch it.
func TestDeviceJSONRoundTrip(t *testing.T) {
	for _, want := range []Device{Push2, Push3} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", want, err)
		}
		var got Device
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", b, err)
		}
		if got != want {
			t.Errorf("round trip %v -> %s -> %v", want, b, got)
		}
	}
}

func TestDeviceMarshalsAsString(t *testing.T) {
	b, err := json.Marshal(Push2)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `"Push 2"` {
		t.Errorf("Marshal(Push2) = %s, want \"Push 2\"", b)
	}
}

func TestDeviceUnmarshalUnknownDefaultsToPush3(t *testing.T) {
	var got Device
	if err := json.Unmarshal([]byte(`"something else"`), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != Push3 {
		t.Errorf("got %v, want Push3 as the default", got)
	}
}

// PortRef itself must round-trip through JSON the same way pushapp-ui's
// Connect decodes it — this is the actual failure mode, one level up from
// Device alone: a struct embedding it, marshalled and unmarshalled the same
// way the frontend does.
func TestPortRefWithDeviceRoundTrips(t *testing.T) {
	type embedding struct {
		Device Device `json:"device"`
	}
	want := embedding{Device: Push3}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got embedding
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal(%s): %v", b, err)
	}
	if got != want {
		t.Errorf("round trip %+v -> %s -> %+v", want, b, got)
	}
}
