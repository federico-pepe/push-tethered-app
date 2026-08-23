# beatcount-py

Process-loaded port of the Go `modules/beatcount`: counts an external MIDI
clock and draws the current beat (1-4) across the pad grid as a digit.

Demonstrates `NeedsMIDIIn` from a process module — a `"handle"`
notification with `"kind": "external_midi"` arrives whenever a byte comes
in on the app's external MIDI input port. Wire gotcha: `ExternalMIDI.Raw`
is a Go `[]byte`, which `encoding/json` encodes as a base64 **string**,
not a JSON array of numbers — `"data": {"raw": "+A=="}` is a single 0xF8
clock tick.

Its screen colors (black, gray, white) come from `palette.json`
(`cmd/genpalette`-generated from `core/push3.Palette`) via
`palette_color` — see
[writing-a-process-module.md](../../../docs/guides/writing-a-process-module.md#colors).

```bash
go run ./cmd/pushapp -install examples/modules/beatcount-py
go run ./cmd/pushapp -module beatcount-py -ext-midi-in "<your clock source>"
```
