# Debugging

**Status:** living guide
**Last verified:** 2026-08-19

## Golden rule

Look at the Push screen. Do not rely on logs alone. A healthy frame rate in
the logs does not prove that the rendering is correct.

## Screen capture

```bash
go run ./cmd/pushapp -capture demo.mp4
go run ./cmd/pushapp -capture-raw frames/   # raw PNG sequence
```

This captures the display output. It does not capture the physical pad LEDs.

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

`pushapp-ui` has no terminal of its own after a user launches it by
double-clicking the app. `log.Printf` output, including every line that
`internal/bootstrap` logs, goes to
`<UserConfigDir>/push-tethered-app/logs/pushapp-ui.log` in addition to
stderr. The host truncates this file on each launch. Ask for this file, or
its contents, when you debug a report from someone who does not run the app
from a terminal.

## No local toolchain for a platform

`.github/workflows/diagnostics.yml` builds `probe`, `frametest`, `mapcheck`,
`pushapp`, and `identifytest` natively per OS in about two minutes. Run it
with `gh workflow run diagnostics.yml`, or from the Actions tab
(`workflow_dispatch` only). It needs no release tag. Use it to get a raw
`-devices` dump from a Windows machine or a Pi with no Go toolchain of its
own.

1. Run the `diagnostics.yml` workflow for the target OS.
2. Download the artifact for that OS.
3. Copy it to the target machine. Use `scp` for a Pi, or a zip download for
   Windows, macOS, or Linux.

macOS-only Swift tools (not part of the Go build):

- `tools/midimon.swift` — MIDI input monitor
- `tools/ledtest.swift` — LED output sweep

Read [protocol/usb-and-safety.md](../protocol/usb-and-safety.md) before you
do a button sweep.

1. Run `pushapp` first. This holds the display claim.
2. Only then are the top-row buttons safe to press.

## Process modules

| Symptom | Likely cause |
|---|---|
| Hangs on start | stdout not flushed (Python) |
| Wrong pad colour | palette index vs RGBA confusion |
| No MIDI out | `needs_midi_out` not set in manifest |
| Empty screen | draw timeout — check child stderr in host log |

The host pipes child stderr to the host log unparsed. Use it for your own
debug prints.

## Live conflict

If the display claim fails with `LIBUSB_ERROR_ACCESS`, Live owns the screen.

1. Quit Live, or release the control surface in Live.
2. Do not replug the device. This step is not necessary.

## ASCII rendering

Non-ASCII text in draw ops renders as missing-glyph boxes. The host also
sanitizes text. Use ASCII in module strings.

## Related

- [plans/2026-08-18-open-items.md](../../plans/2026-08-18-open-items.md) — known gaps
- [guides/writing-a-process-module.md](writing-a-process-module.md)
