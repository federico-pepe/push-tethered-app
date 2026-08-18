# Windows

**Status:** partially verified  
**Last verified:** 2026-08-18  

## Setup

- mingw-w64 toolchain (MSYS2) for cgo
- libusb via MSYS2 or vcpkg
- MIDI: WinMM, built into the vendored RtMidi driver

See [guides/development-setup.md](../guides/development-setup.md).

## Display / USB

**Still untested on real Windows hardware.** CI compiles the binary; WinUSB/Zadig
driver conflicts and WCID descriptors are unknown. MIDI has been exercised on
real hardware; display has not.

## MIDI input — port naming

WinMM does **not** expose USB jack strings like CoreMIDI/ALSA. Push ports appear as:

- `Ableton Push 3 MIDI` (first cable — this is Live Port traffic)
- `MIDIIN2 (Ableton Push 3 MIDI)`, `MIDIIN3 (...)`, etc.

Name-based auto-detect cannot match `"Live Port"`. **Escape hatch:** manual port
selection in `pushapp-ui` when auto-detect fails (`ListInPorts` / `OpenNamed`).

Fix written 2026-08-18 — **not yet confirmed on real Windows hardware** by the
reporting user.

## MIDI output

WinMM **cannot create virtual ports**. The host **attaches** to an existing
port by name:

1. Install [loopMIDI](https://www.tobias-erichsen.de/software/loopmidi.html)
   (free) or use Windows MIDI Services
2. Create a port (e.g. `PushApp`)
3. Point the app at that name (`-midi-out PushApp` or UI setting)

| Platform | MIDI out strategy | User setup |
|---|---|---|
| macOS, Linux | create virtual port | none |
| Windows | attach to existing port | loopMIDI or equivalent |

Never attach to a port whose name mentions **Push** — output loops back into
the input decoder.

## pushapp-ui

Wails v3 builds on Windows CI. Port picker appears when auto-detect fails.

## Related

- [open-questions.md](../open-questions.md) — Windows display untested
- [architecture/module-host.md](../architecture/module-host.md) — MIDI out model
