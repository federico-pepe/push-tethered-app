// Package pushmap holds Push control-surface knowledge that core/push3 does not
// cover: the Push 2 deltas, and device-scoped lookups.
//
// # History
//
// This package used to override core/push3's touch-sensor notes, which were off
// by one for encoders 1-8 and the volume wheel and omitted the touch strip.
// Those corrections were measured on a tethered Push 3, confirmed independently
// on a Push 2, and **have now been applied upstream** (2026-08-16), so the
// overrides are gone and core/push3 is authoritative for Push 3 again.
//
// The `TestDivergesFromCore` tripwire that used to live here did its job: it
// failed the moment upstream was corrected, which is what prompted this
// cleanup rather than leaving a silent permanent fork.
//
// What remains here is genuinely device-specific: Push 2's map deltas (push2.go)
// and the per-device lookups that resolve them.
package pushmap

import "github.com/federico-pepe/ableton-push-hack/core/push3"

// EncoderTouchNote returns the touch note for encoder n (0-indexed, 0-7).
// Thin wrapper over core/push3, kept so callers read symmetrically with the
// device-scoped helpers below.
func EncoderTouchNote(n int) byte { return push3.NoteEncoderTouchN(n) }

// touchNames is the Push 3 touch-sensor table, valued entirely from core/push3.
// Note 9 is deliberately absent: it is unused on Push 3 (it is the Swing
// encoder on Push 2 — see push2.go).
var touchNames = map[byte]string{
	push3.NoteEncoder1Touch: "Encoder 1 touch", push3.NoteEncoder2Touch: "Encoder 2 touch",
	push3.NoteEncoder3Touch: "Encoder 3 touch", push3.NoteEncoder4Touch: "Encoder 4 touch",
	push3.NoteEncoder5Touch: "Encoder 5 touch", push3.NoteEncoder6Touch: "Encoder 6 touch",
	push3.NoteEncoder7Touch: "Encoder 7 touch", push3.NoteEncoder8Touch: "Encoder 8 touch",
	push3.NoteVolumeTouch:     "Volume wheel touch",
	push3.NoteTempoTouch:      "Tempo wheel touch",
	push3.NoteJogTouch:        "Jog wheel touch",
	push3.NoteTouchStrip:      "Touch strip touch",
	push3.NoteDPadCenterTouch: "D-Pad center touch",
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

// TouchNames exposes the table for tools that need to enumerate it.
func TouchNames() map[byte]string {
	out := make(map[byte]string, len(touchNames))
	for k, v := range touchNames {
		out[k] = v
	}
	return out
}
