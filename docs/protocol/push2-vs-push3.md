# Push 2 vs Push 3

**Status:** confirmed on tethered hardware
**Last verified:** 2026-08-16
**Authoritative code:** [internal/pushmap/push2.go](../../internal/pushmap/push2.go)

Both devices run from the **same binary**. Display, pad grid geometry, and
LED palette are identical.

## Summary

| | Push 2 | Push 3 |
|---|---|---|
| USB PID | `0x1967` | `0x2982` / `0x1969` |
| USB interfaces | 3 | 7 |
| MIDI ports (OS) | 2 (Live, User) | 3 (+ External) |
| Audio | none | class-compliant 16×16 |
| `xPort` (interface 6) | absent | present, undocumented |
| MPE on pads | no — always channel 1 | user-configurable (Push's own Aftertouch-mode setting: Polyphonic Aftertouch = channel 1, MPE = channels 2–16 — see [midi-input.md](midi-input.md#mpe)) |
| Button CC map | 75/80 CC + 12/14 touch | 87/87 CC + 13/13 touch |

## Map deltas (Push 2 only)

Five CC assignments differ. Always resolve them with `pushmap` and a
known device:

| CC | Push 2 | Push 3 |
|---|---|---|
| 15 | Swing encoder press | Tempo encoder press |
| 52 | Master | *(same name, verify on hardware)* |
| 53 | Stop Clip | *(same name, verify on hardware)* |
| 87 | New | Push 3 uses CC 92 |
| 111 | Browse | Volume encoder press |

Push 2 **note 9** is the Swing encoder touch. It is unused on Push 3 (a
gap at 9).

Push 2 arrow down/right CCs are **unverified**. They are expected to
match Push 3's 46/47/44/45.

## Code usage

```go
dev := h.Device()  // pushmap.DevicePush2 or DevicePush3
name := pushmap.ButtonNameFor(dev, cc)
touch := pushmap.TouchNameFor(dev, note)
rel := pushmap.IsRelativeEncoderCCFor(dev, cc)
```

Do not use device-agnostic `push3` name helpers when the device is known.

## Related

- [hardware-reference.md](../hardware-reference.md) — full map links
- [midi-input.md](midi-input.md) — MPE and decode notes
- [display.md](display.md) — shared display protocol
</content>
