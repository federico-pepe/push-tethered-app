# Windows

**Status:** verified on real hardware (VM + USB passthrough)  
**Last verified:** 2026-08-19  

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

**Claiming one unit's display can make USB enumeration fail for that same
unit** on real Windows hardware — confirmed 2026-08-19 by a real symptom:
with two Push units attached, pairing one made the *other, completely
unclaimed* unit disappear from `pushapp-ui`'s pairing view too. Root cause
was a bug in `internal/display.enumerateUSB`, not the platform: it looped
Push 3 then Push 2 and aborted the *entire* enumeration if one product
model's every device failed to open — which happens once that device's
display interface is claimed — losing the other, unrelated model's units
along with it. Fixed by letting one model's failure skip ahead to the next
rather than aborting; a caller only sees an error when nothing was found at
all. Whether opening a *second* device-level handle to an
interface-claimed device is what actually fails on Windows (as opposed to
some other OS-specific enumeration behavior) was not independently
confirmed — the loop bug was the reproducible, code-provable part, and
fixing it resolved the live symptom regardless of the deeper cause.

## MIDI input — port naming

WinMM does **not** expose USB jack strings like CoreMIDI/ALSA. Push ports appear as:

- `Ableton Push 3 MIDI` (first cable — this is Live Port traffic)
- `MIDIIN2 (Ableton Push 3 MIDI)`, `MIDIIN3 (...)`, etc.

Name-based auto-detect cannot match `"Live Port"`. **Escape hatch:** manual port
selection in `pushapp-ui` when auto-detect fails (`ListInPorts` / `OpenNamed`).

**This driver's Windows backend appends an undocumented `" <n>"` to every
MIDI port name it reports** — not just Push's, and confirmed to match each
port's own driver number (`PortRef.InNum`/`OutNum`), incrementing globally
across every Push-named port on the system rather than resetting per unit.
Measured live 2026-08-19 on real Windows hardware:

```
Ableton Push 3 MIDI 0
MIDIIN2 (Ableton Push 3 MIDI) 1
MIDIIN3 (Ableton Push 3 MIDI) 2
```

(a second Push attached at the same time continued the same global counter —
`Ableton Push 2 0`, `MIDIIN2 (Ableton Push 2) 1` became `... 2`, `... 3` once
a Push 3 was also present). This is not decoration a caller can ignore: it
broke role detection outright (every cable showed as an unnamed "cable 1,
Live" since the name no longer ended in a recognisable shape) and broke
cable-2-and-up detection too (the wrapped-name regex anchors to the closing
paren, and the trailing index sits after it). `internal/midi.unitKeyOf`
strips this suffix (`winmmIndex`) before classifying a name, the same way it
already stripped ALSA's `<client>:<port>` suffix on Linux.

Separately, and independent of the suffix above: WinMM numbers MIDI in and
out cables in namespaces that are **entirely independent of each other** —
another MIDI-out device already on the system (a synth, a loopMIDI port) can
shift Push's own outputs to different absolute cable numbers than its
inputs, so even a cable whose input and output *should* share a name no
longer do once decorated. `internal/midi.groupPorts` pairs cables by
**position within the unit** (the Nth remaining input against the Nth
remaining output of that same physical unit, each ordered by its own cable
number) rather than by matching an absolute cable number between sides —
this survives both the suffix and the independent-numbering problem at once.
Confirmed live 2026-08-19 on real Windows hardware, one Push 3 and one Push
2, together and separately: every cable paired correctly and the pairing UI
worked end to end. (A same-shaped 2026-08-18 fix, keyed on the absolute
cable number rather than relative position, is what this superseded — it
worked for the simpler cases that fix was measured against, but not this
one.)

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
