# Display protocol

**Status:** confirmed on tethered hardware  
**Last verified:** 2026-08-09 (Push 3, macOS)  
**Authoritative code:** [internal/display/display.go](../../internal/display/display.go), `core/display`, `core/push3`

Push 2 and Push 3 share the same display protocol in tethered mode. Evidence
trail: [archive/feasibility.md](../archive/feasibility.md) §1, §8.3.

## USB

| Property | Value |
|---|---|
| Interface to claim | **0** (vendor-specific, 2 endpoints) |
| Bulk OUT endpoint | `0x01` |
| Push 2 USB PID | `0x1967` |
| Push 3 USB PID | `0x2982` / `0x1969` |

Claim **only interface 0**. Claiming MIDI or audio interfaces removes them
from the OS. See [usb-and-safety.md](usb-and-safety.md).

Endpoint `0x81` IN on the display interface is unused by the write path —
possibly a status/ack channel; never read from speculatively.

## Frame format

| Property | Value |
|---|---|
| Header | 16 bytes: `FF CC AA 88` + 12 × `00` |
| Visible size | 960 × 160 px |
| Row stride | 1024 px (64 px padding per row) |
| Pixel format | BGR565 little-endian |
| Line XOR | `0xFFE7F3E7` (bytes `{E7,F3,E7,FF}` per row) |
| Payload size | **327680 bytes** (one frame) |

A single 327680-byte frame is sufficient. The standalone Push 3 app duplicates
each frame per update — that is a quirk of Ableton's binary, not a hardware
requirement ([archive/feasibility.md](../archive/feasibility.md) §8.3).

Encoding: `core/display.ToBGR565` produces the exact USB payload.

## Refresh

The screen **must be refreshed continuously**. With no host driving it, Push
redraws its own idle screen over whatever was last sent.

- **30 fps** (~9.4 MB/s) runs clean on measured hardware
- **60 fps** is within USB 2.0 budget

Use `pushapp -capture out.mp4` to record the screen for inspection (physical
pad LEDs are not captured).

## Disconnect while running

Confirmed 2026-08-20: unplugging Push mid-session surfaces as a failed frame
write (`writing frame header: transfer failed`), which `internal/display`
turns into `ErrDisconnected`; the host logs `host: Push disconnected` and
exits — no crash, no wedge. `pushapp` does not auto-relaunch or reclaim on
replug; that needs a fresh launch. If Live is running and was never
unplugged (only the device was), Live's still-alive background helper (see
[usb-and-safety.md](usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20))
reclaims interface 0 on replug automatically, with no arbitration needed
since `pushapp` had already exited.

## Probes

```bash
go run ./cmd/frametest          # one frame or timed hold
go run ./cmd/probe              # USB descriptor dump (read-only)
go run ./cmd/pushapp -capture demo.mp4
```

## Related

- [push2-vs-push3.md](push2-vs-push3.md) — USB interface count differs
- [usb-and-safety.md](usb-and-safety.md) — claim rules, Live exclusivity
- [Ableton/push-interface](https://github.com/Ableton/push-interface) —
  official Push 2 spec (geometry matches Push 3 tethered)
