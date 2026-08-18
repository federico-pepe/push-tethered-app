# Debugging

**Status:** living guide  
**Last verified:** 2026-08-18  

## Golden rule

**Look at the Push screen, not just the logs.** A healthy frame rate in logs
does not guarantee correct rendering.

## Screen capture

```bash
go run ./cmd/pushapp -capture demo.mp4
go run ./cmd/pushapp -capture-raw frames/   # raw PNG sequence
```

Captures the **display output**, not physical pad LEDs.

## Display-only probe

```bash
go run ./cmd/frametest     # one frame or timed hold, no module
go run ./cmd/probe         # USB descriptor dump (never opens device)
```

Protocol: [protocol/display.md](../protocol/display.md).

## MIDI

```bash
go run ./cmd/midiouttest -list
go run ./cmd/midiouttest -port "PushApp" -listen "PushApp"  # loopback test
go run ./cmd/mapcheck      # cross-reference capture vs button map
```

macOS-only Swift tools (not part of Go build):

- `tools/midimon.swift` — MIDI input monitor
- `tools/ledtest.swift` — LED output sweep

Read [protocol/usb-and-safety.md](../protocol/usb-and-safety.md) before button
sweeps. **Hold the display first** — run `pushapp` so top-row buttons are safe.

## Process modules

| Symptom | Likely cause |
|---|---|
| Hangs on start | stdout not flushed (Python) |
| Wrong pad colour | palette index vs RGBA confusion |
| No MIDI out | `needs_midi_out` not set in manifest |
| Empty screen | draw timeout — check child stderr in host log |

Child stderr is piped to the host log unparsed — use it for your own debug
prints.

## Live conflict

If display claim fails with `LIBUSB_ERROR_ACCESS`, Live owns the screen. Quit
Live or release the control surface — no replug needed.

## ASCII rendering

Non-ASCII text in draw ops renders as missing-glyph boxes. The host also
sanitises text. Use ASCII in module strings.

## Related

- [open-questions.md](../open-questions.md) — known gaps
- [guides/writing-a-process-module.md](writing-a-process-module.md)
