# Debugging

**Status:** living guide  
**Last verified:** 2026-08-19  

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
go run ./cmd/pushapp -devices   # every attached Push unit + MIDI cable, claims nothing
```

## `pushapp-ui` log file

`pushapp-ui` has no terminal of its own once launched by double-clicking the
app — `log.Printf` output (including every line `internal/bootstrap` logs)
goes to `<UserConfigDir>/push-tethered-app/logs/pushapp-ui.log` in addition
to stderr, truncated fresh on each launch. Ask for this file, or its
contents, when debugging a report from someone who isn't running the app
from a terminal.

## No local toolchain for a platform

`.github/workflows/diagnostics.yml` (`gh workflow run diagnostics.yml`, or
the Actions tab, `workflow_dispatch` only) builds
`probe`/`frametest`/`mapcheck`/`pushapp`/`identifytest` natively per OS in
about two minutes — no release tag needed. Used to get a raw `-devices` dump
from a Windows machine and a Pi with no Go toolchain of its own; download
the artifact for the OS in question and copy it over (`scp` to a Pi, a zip
download for Windows/macOS/Linux).

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

- [plans/2026-08-18-open-items.md](../../plans/2026-08-18-open-items.md) — known gaps
- [guides/writing-a-process-module.md](writing-a-process-module.md)
