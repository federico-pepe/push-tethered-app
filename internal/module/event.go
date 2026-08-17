package module

// Event is one decoded input from Push.
//
// These deliberately mirror internal/midi's event types rather than reusing
// them. midi.Event is sealed with an unexported method — correct for a decoder,
// since nothing outside that package should invent a decoded event — but it
// makes the type unusable as an ABI: a module running as a separate process has
// to be able to *reconstruct* events, and a sealed interface forbids that.
//
// So the decoder keeps its sealed types and the host translates. The cost is
// five small structs; the gain is a wire-ready contract that does not change
// when the decoder does.
//
// Names are resolved by the host before the event arrives (button names, encoder
// names) because resolving them needs the per-device tables in internal/pushmap,
// and a module should not have to care that CC 15 is a Swing encoder on Push 2
// and a Tempo press on Push 3.
type Event interface {
	// EventKind returns a short stable discriminator: "pad", "button",
	// "encoder", "touch", "expression". Named EventKind rather than Kind
	// because Expression already has a Kind field.
	EventKind() string
}

// Pad is a grid pad press or release. Col and Row are 0-indexed from the
// bottom-left, matching push3.PadCoord.
//
// Channel matters: MPE is on by default but not always, so pad note-ons may
// arrive on channel 1 or rotate across channels 2-16 with nothing deliberately
// changed between sessions. Never assume one layout.
type Pad struct {
	Note     byte `json:"note"`
	Col      int  `json:"col"`
	Row      int  `json:"row"`
	Channel  int  `json:"channel"`
	Velocity byte `json:"velocity"`
	Pressed  bool `json:"pressed"`
}

func (Pad) EventKind() string { return "pad" }

// Button is a press or release of a CC button on channel 1. Name is empty for a
// CC with no entry in the map for this device.
type Button struct {
	CC      byte   `json:"cc"`
	Name    string `json:"name,omitempty"`
	Pressed bool   `json:"pressed"`
}

func (Button) EventKind() string { return "button" }

// Encoder is a relative encoder movement, already decoded to a signed delta.
//
// Delta is not limited to +/-1: Push's encoders accelerate, and deltas up to
// +/-11 have been measured on fast turns. Accumulate it, never count messages.
// Index is 0-7 for the eight screen encoders and -1 for the others (volume,
// tempo, jog).
type Encoder struct {
	CC    byte   `json:"cc"`
	Index int    `json:"index"`
	Delta int    `json:"delta"`
	Name  string `json:"name,omitempty"`
}

func (Encoder) EventKind() string { return "encoder" }

// Touch is a capacitive touch sensor making or breaking contact — the encoders,
// the volume and tempo knobs, the jog wheel, the touch strip, the D-Pad centre.
type Touch struct {
	Note    byte   `json:"note"`
	Name    string `json:"name,omitempty"`
	Touched bool   `json:"touched"`
}

func (Touch) EventKind() string { return "touch" }

// Expression is per-note MPE data: aftertouch pressure, CC 74 slide, or pitch
// bend, on the note's own member channel.
//
// This is high-rate. A module that ignores it costs nothing; a module that logs
// every one will flood.
type Expression struct {
	Channel int    `json:"channel"`
	Kind    string `json:"kind"` // "pressure" | "slide" | "bend"
	Value   int    `json:"value"`
}

func (Expression) EventKind() string { return "expression" }
