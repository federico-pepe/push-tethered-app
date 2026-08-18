# Windows

**Status:** verified on real hardware (VM + USB passthrough)  
**Last verified:** 2026-08-18  

## Setup

- mingw-w64 toolchain (MSYS2) for cgo
- libusb via MSYS2 or vcpkg
- MIDI: WinMM, built into the vendored RtMidi driver

See [guides/development-setup.md](../guides/development-setup.md).

## Display / USB

**Confirmed 2026-08-18** in a Windows 11 VM with a real Push 3 attached via
USB passthrough: `pushapp-ui` ran end to end, display and MIDI both working.
No WinUSB/Zadig driver conflict encountered; whether Push advertises WCID/MS
OS descriptors specifically was not investigated since the plain path already
worked.

## MIDI input — port naming

WinMM does **not** expose USB jack strings like CoreMIDI/ALSA. Push ports appear as:

- `Ableton Push 3 MIDI` (first cable — this is Live Port traffic)
- `MIDIIN2 (Ableton Push 3 MIDI)`, `MIDIIN3 (...)`, etc.

Name-based auto-detect cannot match `"Live Port"`. **Escape hatch:** manual port
selection in `pushapp-ui` when auto-detect fails (`ListInPorts` / `OpenNamed`).

This broke `OpenNamed` too, not just auto-detect: it opened MIDI in and out by
the *same literal string*, but WinMM numbers in/out cables independently, so
an input like `MIDIIN2 (Ableton Push 3 MIDI)` has no output port sharing that
name — manual picking failed with `can't find MIDI output port for ...`.
Fixed 2026-08-18: `OpenNamed` now falls back to matching the output cable by
*position* among Push-named ports (same index as the chosen input among
Push-named inputs), since both lists enumerate in the device's own cable
order. Confirmed against a real Windows report, and re-verified 2026-08-18
against real Push 3 hardware (VM + USB passthrough) — MIDI connected
successfully as part of that end-to-end run.

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

### Missing DLL errors at runtime

A Windows exe built via MSYS2/mingw dynamically links four DLLs by default:
`libgcc_s_seh-1.dll`, `libstdc++-6.dll`, `libwinpthread-1.dll` (mingw
runtime), and `libusb-1.0.dll` (gousb's `#cgo pkg-config: libusb-1.0`). On
the build machine they're on PATH via MSYS2, so the build succeeds — but
copying just the `.exe` to another Windows machine fails to launch with
"missing DLL" errors for all four.

**Do not source these DLLs from elsewhere** (a random download, a different
MSYS2 install, a different mingw build) to paper over a missing-DLL error.
A runtime DLL built against a different toolchain version than the exe
produces `0xc000007b` (`STATUS_INVALID_IMAGE_FORMAT`) instead — confirmed
2026-08-18 against a VM report after manually sourcing
`libusb-1.dll`/`libwinpthread-1.dll`. The fix is to make the exe not need
external copies of these DLLs at all.

Fix, applied 2026-08-18 in response to that VM report and confirmed working
the same day against real Push 3 hardware (same VM + USB passthrough):

- `libgcc_s_seh-1.dll` / `libstdc++-6.dll` / `libwinpthread-1.dll`:
  static-link with `CGO_LDFLAGS="-static"` (a plain `-static-libgcc
  -static-libstdc++` misses `libwinpthread-1.dll`) — MSYS2's toolchain ships
  static archives for all three, safe by default. CI does this for
  `cmd/pushapp-ui`.
- `libusb-1.0.dll`: static-linking this needs `libusb-1.0.a` present in
  MSYS2's `mingw64/lib`, unverified on real hardware — simpler and
  confirmed-working fix is to ship `mingw64/bin/libusb-1.0.dll` alongside
  the exe, always from the *same* MSYS2 install the exe was built with. CI
  copies it next to `cmd/pushapp-ui/bin/pushapp-ui.exe`.

Locally, in the MSYS2 shell before building:

```bash
export PATH="/mingw64/bin:$PATH"
export CGO_LDFLAGS="-static"
cd cmd/pushapp-ui && wails3 task build CGO_ENABLED=1
cp /mingw64/bin/libusb-1.0.dll bin/
```

This isn't packaging (`wails3 package`, NSIS) — still out of scope per
[plans/2026-08-17-ci-for-pushapp-ui.md](../../plans/2026-08-17-ci-for-pushapp-ui.md).
An installer would bundle this automatically; until then, ship the DLL by hand.

## Related

- [plans/2026-08-18-open-items.md](../../plans/2026-08-18-open-items.md) — remaining open items
- [architecture/module-host.md](../architecture/module-host.md) — MIDI out model
