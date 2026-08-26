# Display protocol

**Status:** confirmed on tethered hardware
**Last verified:** 2026-08-20 (Push 3, macOS)
**Authoritative code:** [internal/display/display.go](../../internal/display/display.go), `core/display`, `core/push3`

Push 2 and Push 3 use the same display protocol in tethered mode. Evidence
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

Endpoint `0x81` IN on the display interface is not used by the write path.
It is possibly a status or acknowledgment channel. Do not read from it
without a specific reason.

## Frame format

| Property | Value |
|---|---|
| Header | 16 bytes: `FF CC AA 88` + 12 × `00` |
| Visible size | 960 × 160 px |
| Row stride | 1024 px (64 px padding per row) |
| Pixel format | BGR565 little-endian |
| Line XOR | `0xFFE7F3E7` (bytes `{E7,F3,E7,FF}` per row) |
| Payload size | **327680 bytes** (one frame) |

A single 327680-byte frame is sufficient. The standalone Push 3 app
duplicates each frame for every update. This is a quirk of Ableton's
binary, not a hardware requirement (see
[archive/feasibility.md](../archive/feasibility.md) §8.3).

Encoding: `core/display.ToBGR565` produces the exact USB payload.

## Refresh

The screen **must be refreshed continuously**. With no host driving it,
Push redraws its own idle screen over whatever was last sent.

- **30 fps** (~9.4 MB/s) produces no errors on measured hardware.
- **60 fps** stays within the USB 2.0 bandwidth budget.

Use `pushapp -capture out.mp4` to record the screen for inspection
(physical pad LEDs are not captured).

## Disconnect while running

Confirmed on 2026-08-20: when Push disconnects mid-session, the frame
write fails with `writing frame header: transfer failed`.
`internal/display` converts this error into `ErrDisconnected`. The host
logs `host: Push disconnected` and exits. There is no crash and no wedge.

`pushapp` does not relaunch itself and does not reclaim the device
automatically after a replug. A fresh launch is necessary to reclaim it.

If Live is running and only the device was unplugged (not Live), Live's
background helper stays alive (see
[usb-and-safety.md](usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20)).
This helper reclaims interface 0 automatically on replug. No arbitration
is necessary, because `pushapp` already exited.

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
</content>
</invoke>
