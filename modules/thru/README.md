# thru

Forwards Push's controls straight out as MIDI: press a pad, another app
receives a note; turn an encoder, it receives a CC. It's the smallest
module that actually sends MIDI, proving the whole output path —
module → host → `internal/midiout` → a port other software can see.

No MPE (per-note channel/pressure/slide/bend all collapse onto one output
channel) and no configuration (output channel is fixed) — predictable
output beats faithful output for something whose job is to be verifiable.
It's also the identity case of a remapper: `modules/remap`'s default
behaviour with no rules defined is exactly this module.

```bash
go run ./cmd/pushapp -module thru
```
