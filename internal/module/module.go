// Package module is the ABI between the host and a module.
//
// A module is a small program that owns Push's screen and controls while it is
// active: it draws a frame, it handles input, and it can send MIDI out to other
// software. Modules do not touch USB, do not open MIDI ports and do not know
// what a frame buffer is — the host owns all of that.
//
// Three properties are deliberate, and each one is load-bearing:
//
//  1. **A module never draws pixels.** Draw builds a display list (see Frame)
//     which the host renders with core/gfx and core/gfx/widgets. That keeps the
//     look consistent, keeps rendering cheap on weak hardware, lets the host
//     apply a theme, and — the reason it matters most — makes the whole contract
//     serialisable, so a later loader can run a module as a separate process in
//     any language.
//
//  2. **Handle and Draw are never called concurrently.** The host serialises
//     both onto one goroutine. Module authors need no mutexes. This is not an
//     implementation detail to rely on loosely: it is part of the contract.
//
//  3. **The op set is open.** core/gfx and core/gfx/widgets are expected to grow
//     new components, so ops are name-plus-payload rather than a closed enum,
//     and the host renders them from a registry. Adding a widget is additive:
//     it breaks no existing module and needs no version bump.
//
// Types that describe *drawing* are aliases of the upstream core/gfx/widgets
// types rather than copies. Widgets live upstream in ableton-push-hack; this
// package must not fork them, and an alias means an upstream addition shows up
// here for free.
package module

import (
	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/push-tethered-app/internal/pushmap"
)

// Drawing types, aliased from upstream. Aliases, not definitions: a module
// passing a widgets.ListView and a module passing a module.ListView are passing
// the identical type, so nothing needs converting and upstream stays the single
// source of truth.
type (
	Theme           = widgets.Theme
	ListView        = widgets.ListView
	ListRow         = widgets.ListRow
	HListView       = widgets.HListView
	KVRow           = widgets.KVRow
	SoftButton      = widgets.SoftButton
	SoftButtonState = widgets.SoftButtonState
	Knob            = widgets.Knob
	Weight          = text.Weight
)

// Weight values, aliased from core/gfx/text — see StyledText.
const (
	Regular    = text.Regular
	Bold       = text.Bold
	Italic     = text.Italic
	BoldItalic = text.BoldItalic
)

// Meta identifies a module to the host and to the user.
type Meta struct {
	ID          string `json:"id"`   // stable, filename-safe; used by -module and by config
	Name        string `json:"name"` // shown to the user
	Author      string `json:"author,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"` // one line, shown under Name in the module list

	// NeedsMIDIOut declares that this module sends MIDI to other software.
	// The host refuses to activate it when no output port is available, rather
	// than letting every SendCC fail silently at runtime.
	NeedsMIDIOut bool `json:"needs_midi_out,omitempty"`

	// NeedsMIDIIn declares that this module wants ExternalMIDI events — raw
	// MIDI from other software or hardware, not from Push (see
	// internal/midiin, module.ExternalMIDI). Unlike NeedsMIDIOut this does
	// NOT refuse activation when no input port is available: an external
	// clock or controller is an enhancement a module can do without, not
	// something that makes it non-functional the way a MIDI-out module with
	// no output would be. The module simply never receives ExternalMIDI
	// events in that case; the host logs why once.
	NeedsMIDIIn bool `json:"needs_midi_in,omitempty"`
}

// Module is the contract. Every method is called from the host's single module
// goroutine, so implementations need no locking.
type Module interface {
	// Meta identifies the module. Must be constant and safe to call at any
	// time, including before Init.
	Meta() Meta

	// Init runs once when the module becomes active. Hold on to h — it is the
	// only way to reach the hardware.
	Init(h Host) error

	// Handle receives one input event. Return quickly: the same goroutine draws
	// frames, so blocking here stalls the display.
	Handle(ev Event)

	// Draw appends this frame's display list to f, which arrives empty.
	Draw(f *Frame)

	// Close runs when the module is deactivated or the app shuts down. The host
	// clears all LEDs afterwards, so there is no need to do it here.
	Close() error
}

// Host is what a module is allowed to do. The host implements it; a fake
// implementation makes modules testable with no hardware attached.
type Host interface {
	// Device reports which Push this is. Never hidden from modules: two CCs
	// mean different things per device (CC 15, CC 111), so a module that maps
	// controls has to know.
	Device() pushmap.Device

	// Theme is the palette the host renders with. Use it instead of literal
	// colours so modules look like they belong together.
	Theme() Theme

	// SetPad lights a grid pad. colour is a palette index from
	// core/push3/colors.go; 0 is off.
	SetPad(note, colour byte)

	// SetButton lights a button LED. value is a palette index, same mechanism
	// and same palette as SetPad's colour — confirmed 2026-08-18 on the
	// screen-adjacent buttons (see docs/protocol/led-output.md); other button
	// classes are unmeasured and may genuinely be brightness-only. 0 is off.
	SetButton(cc, value byte)

	// SendCC, SendNote and NoteOff reach other software through the host's
	// output port. ch is 1-16. All three return an error if the module did not
	// declare NeedsMIDIOut, or if no port was available.
	SendCC(ch, cc, val byte) error
	SendNote(ch, note, vel byte) error
	NoteOff(ch, note byte) error

	// SendClock, SendStart, SendContinue and SendStop send MIDI timing
	// clock/transport messages through the same output port as SendCC — same
	// NeedsMIDIOut requirement, no channel (these are system realtime
	// messages). Send SendClock 24 times per quarter note to drive another
	// device's tempo.
	SendClock() error
	SendStart() error
	SendContinue() error
	SendStop() error

	// SupportedOps lists the display-list ops this host can render. A module
	// built against a newer core/gfx can degrade instead of drawing nothing.
	SupportedOps() []string

	// Log writes a line to the app's log, tagged with the module's ID.
	Log(format string, args ...any)

	// Store persists this module's own settings.
	Store() Store
}

// Store is per-module persistence. The host owns the file, the format and the
// atomic write; a module only sees its own document.
type Store interface {
	// Get unmarshals the stored document into v. It is not an error for
	// nothing to have been stored yet — v is left untouched in that case, so
	// defaults set before the call survive.
	Get(v any) error

	// Set marshals v and stores it.
	Set(v any) error
}
