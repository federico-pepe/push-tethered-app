# Hardware reference — upstream links and code pointers

**Status:** living index  
**Last verified:** 2026-08-18  
**Authoritative code:** `core/push3`, `internal/pushmap`

Push button maps, LED palette tables, and geometry constants are shared with
the sibling project [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack)
through the `core/` Go module. **Do not duplicate full tables here.** Link
to the upstream document. Note tethered-specific deltas locally.

## Upstream docs (ableton-push-hack)

| Topic | Document |
|---|---|
| Button / encoder / pad MIDI map | [push3-button-map.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-button-map.md) |
| 128-entry LED colour palette | [push3-led-colors.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-led-colors.md) |
| Push 3 standalone OS internals | [push3-internals.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-internals.md) |

This project re-verified these facts on **tethered** Push 2 and Push 3
hardware. See [archive/feasibility.md](archive/feasibility.md) §8–§12.

## Authoritative in code

| What | Where |
|---|---|
| Display geometry, encoder decode, palette indices | `core/push3` (upstream) |
| Push 2 CC/touch deltas (5 differing CCs, note 9 = Swing touch) | [internal/pushmap/push2.go](../internal/pushmap/push2.go) |
| Device-aware name lookup | `pushmap.ButtonNameFor`, `TouchNameFor`, `IsRelativeEncoderCCFor` |
| Tethered USB display transport | [internal/display/](../internal/display/) |
| OS MIDI decode | [internal/midi/](../internal/midi/) |

Always resolve button and touch names **per device** with `pushmap`. Never
assume Push 3 CC numbers on a Push 2.

## Tethered-specific docs (this repo)

Facts that differ from standalone Push 3 or from upstream docs alone:

- [protocol/display.md](protocol/display.md) — USB interface 0, frame format
- [protocol/midi-input.md](protocol/midi-input.md) — OS MIDI ports, MPE behaviour
- [protocol/led-output.md](protocol/led-output.md) — CoreMIDI LED path
- [protocol/push2-vs-push3.md](protocol/push2-vs-push3.md) — interface and map deltas
