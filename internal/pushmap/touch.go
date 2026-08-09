// Package pushmap holds Push 3 control-surface map corrections measured in
// tethered (controller) mode that differ from the shared core/push3 map.
//
// core/push3 is authoritative for everything else — pads, button CCs, encoder
// CCs, the LED palette and DecodeRel — and this package deliberately does NOT
// re-export those. Import core/push3 directly for them.
//
// # Why this exists
//
// The touch-sensor note numbers in core/push3 (and in that project's
// docs/push3-button-map.md) are off by one for encoders 1-8 and the volume
// wheel, and they omit the touch strip entirely. Measured 2026-08-09 on a
// tethered Push 3 by touching each sensor in a known order, 60s capture, no
// turning — see docs/feasibility.md §8.8.
//
//	sensor           measured   core/push3 says
//	encoder 1-8      0-7        1-8              off by one
//	volume wheel     8          9                off by one
//	(note 9)         unused     -                gap the old map missed
//	tempo wheel      10         10               correct
//	jog wheel        11         11               correct
//	touch strip      12         (absent)         undocumented
//	D-Pad center     13         13               correct
//
// The old numbering looks like it assumed a contiguous 1..10 run for the eight
// encoders plus both wheels. The unused note 9 is exactly what that assumption
// would paper over.
//
// # Why it is not fixed upstream
//
// A deliberate call: core/push3 is shared with ableton-push-hack, whose
// standalone hacks were built against the current values, and that project's
// map doc claims its own empirical verification. Rather than change shared
// constants on the strength of a tethered-only measurement, the correction
// lives here. If the standalone device is ever re-measured and agrees, fold
// this into core/push3 and delete this package.
package pushmap

import "github.com/federico-pepe/ableton-push-hack/core/push3"

// Touch-sensor notes, channel 1. Note On velocity 127 = contact, Note Off (or
// velocity 0) = release. These supersede the core/push3 Note*Touch constants.
const (
	NoteEncoder1Touch = 0
	NoteEncoder2Touch = 1
	NoteEncoder3Touch = 2
	NoteEncoder4Touch = 3
	NoteEncoder5Touch = 4
	NoteEncoder6Touch = 5
	NoteEncoder7Touch = 6
	NoteEncoder8Touch = 7

	NoteVolumeTouch = 8
	// note 9 is not emitted by any sensor we have exercised.
	NoteTempoTouch      = 10
	NoteJogTouch        = 11
	NoteTouchStripTouch = 12 // absent from core/push3
	NoteDPadCenterTouch = 13
)

// EncoderTouchNote returns the touch note for encoder n (0-indexed, 0-7).
// Supersedes push3.NoteEncoderTouchN, which is off by one.
func EncoderTouchNote(n int) byte { return byte(NoteEncoder1Touch + n) }

// touchNames is the corrected lookup for non-pad channel-1 notes.
var touchNames = map[byte]string{
	NoteEncoder1Touch: "Encoder 1 touch", NoteEncoder2Touch: "Encoder 2 touch",
	NoteEncoder3Touch: "Encoder 3 touch", NoteEncoder4Touch: "Encoder 4 touch",
	NoteEncoder5Touch: "Encoder 5 touch", NoteEncoder6Touch: "Encoder 6 touch",
	NoteEncoder7Touch: "Encoder 7 touch", NoteEncoder8Touch: "Encoder 8 touch",
	NoteVolumeTouch:     "Volume wheel touch",
	NoteTempoTouch:      "Tempo wheel touch",
	NoteJogTouch:        "Jog wheel touch",
	NoteTouchStripTouch: "Touch strip touch",
	NoteDPadCenterTouch: "D-Pad center touch",
}

// TouchName returns the sensor name for a channel-1 non-pad note, and whether
// it is known. Pad notes (36-99) are not handled here — use push3.IsPadNote.
func TouchName(note byte) (string, bool) {
	if push3.IsPadNote(note) {
		return "", false
	}
	n, ok := touchNames[note]
	return n, ok
}

// TouchNames exposes the corrected table for tools that need to enumerate it.
func TouchNames() map[byte]string {
	out := make(map[byte]string, len(touchNames))
	for k, v := range touchNames {
		out[k] = v
	}
	return out
}
