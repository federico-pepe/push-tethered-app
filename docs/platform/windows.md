# Windows

**Status:** verified on real hardware (VM + USB passthrough)  
**Last verified:** 2026-08-19  

## Setup

- mingw-w64 toolchain (MSYS2) for cgo
- libusb via MSYS2 or vcpkg
- MIDI: WinMM, built into the vendored RtMidi driver

See [guides/development-setup.md](../guides/development-setup.md).

## Display / USB

The team confirmed this on 2026-08-18, in a Windows 11 VM with a real
Push 3 attached through USB passthrough. `pushapp-ui` ran end to end,
with both the display and MIDI working. The team found no WinUSB/Zadig
driver conflict. The team did not investigate whether Push advertises
WCID/MS OS descriptors specifically, because the plain path already
worked.

On real Windows hardware, claiming one unit's display can make USB
enumeration fail for that same unit. The team confirmed this on
2026-08-19 with a real symptom. With two Push units attached, pairing
one unit made the other, completely unclaimed, unit disappear from
`pushapp-ui`'s pairing view too. The root cause was a bug in
`internal/display.enumerateUSB`, not the platform. The function looped
through Push 3, then Push 2, and aborted the entire enumeration if every
device of one product model failed to open. This failure happens once
the display interface of that device is claimed. As a result, the loop
also lost the units of the other, unrelated model. The fix lets one
model's failure skip ahead to the next model, instead of aborting. Now a
caller sees an error only when the enumeration finds nothing at all. The
team did not independently confirm whether opening a second
device-level handle to an interface-claimed device is the real cause of
the Windows failure, as opposed to some other OS-specific enumeration
behavior. The loop bug was the reproducible, code-provable part. Fixing
it resolved the live symptom, regardless of the deeper cause.

## MIDI input — port naming

WinMM does not expose USB jack strings the way CoreMIDI and ALSA do.
Push ports appear as:

- `Ableton Push 3 MIDI` (first cable — this is Live Port traffic)
- `MIDIIN2 (Ableton Push 3 MIDI)`, `MIDIIN3 (...)`, and so on

Name-based auto-detect cannot match `"Live Port"`. Escape hatch: if
auto-detect fails, select the port manually in `pushapp-ui`
(`ListInPorts` / `OpenNamed`).

This driver's Windows backend appends an undocumented `" <n>"` suffix to
every MIDI port name it reports, not only Push's. The team confirmed
that this number matches each port's own driver number
(`PortRef.InNum`/`OutNum`). This number increases globally across every
Push-named port on the system, rather than resetting for each unit. The
team measured this live on real Windows hardware on 2026-08-19:

```
Ableton Push 3 MIDI 0
MIDIIN2 (Ableton Push 3 MIDI) 1
MIDIIN3 (Ableton Push 3 MIDI) 2
```

A second Push attached at the same time continues the same global
counter: `Ableton Push 2 0` and `MIDIIN2 (Ableton Push 2) 1` become
`... 2` and `... 3` once a Push 3 is also present. A caller cannot
ignore this suffix. It broke role detection completely, because the
name no longer ended in a recognizable shape, so every cable showed as
an unnamed "cable 1, Live". It also broke detection of cable 2 and
higher, because the wrapped-name regex anchors to the closing
parenthesis, and the trailing index sits after it.
`internal/midi.unitKeyOf` strips this suffix (`winmmIndex`) before it
classifies a name, the same way it already strips ALSA's
`<client>:<port>` suffix on Linux.

Separately, and independent of the suffix, WinMM numbers MIDI in and out
cables in namespaces that are entirely independent of each other.
Another MIDI-out device already on the system (a synth, a loopMIDI
port) can shift Push's own outputs to absolute cable numbers different
from its inputs. As a result, a cable whose input and output should
share a name no longer does, once the suffix decorates it.
`internal/midi.groupPorts` pairs cables by position within the unit
instead: the Nth remaining input against the Nth remaining output of
the same physical unit, each ordered by its own cable number. This
method survives both the suffix and the independent-numbering problem
at the same time. The team confirmed this live on 2026-08-19 on real
Windows hardware, with one Push 3 and one Push 2, together and
separately. Every cable paired correctly, and the pairing UI worked end
to end. This method superseded a same-shaped fix from 2026-08-18 that
used the absolute cable number instead of relative position. That
earlier fix worked for simpler cases, but not for this one.

## MIDI output

WinMM **cannot create virtual ports**. The host **attaches** to an
existing port by name:

1. Install [loopMIDI](https://www.tobias-erichsen.de/software/loopmidi.html)
   (free), or use Windows MIDI Services (see below — confirmed to work as
   of the 2026-08-27 fixes). loopMIDI has no ARM64 build; on Windows on
   ARM, Windows MIDI Services is the only option.
2. Name the port `Push Tethered App`, matching `midiout.DefaultName`, so the
   app finds it with no flag. `pushapp-ui` has no MIDI-out name field yet —
   only `cmd/pushapp`'s `-midi-out <name>` flag can point at a different name.

| Platform | MIDI out strategy | User setup |
|---|---|---|
| macOS, Linux | create virtual port | none |
| Windows | attach to existing port | loopMIDI, Windows MIDI Services, or equivalent |

Do not attach to a port whose name mentions **Ableton Push** — the real
hardware marker (`internal/midiout.hardwareNameMarker`,
`internal/midi.hardwareNameMarker`, `internal/midiin.hardwareNameMarker`).
A bare "push" substring is not enough: the app's own default port names —
`Push Tethered App` (out) and `Push Tethered App In` (in) — contain it, so
a looser filter self-excludes the very port a Windows user names to match
those defaults. Fixed 2026-08-27; before the fix, `attach()` could never
find a correctly-named loopback port on Windows at all, on either side.

### RtMidi on Windows silently fakes success creating a virtual port

Confirmed live 2026-08-27, on a Windows-on-ARM machine with a real Push 3.
The vendored RtMidi C++ library (`gitlab.com/gomidi/midi/v2/drivers/rtmididrv`)
reports its Windows refusal —
`MidiOutWinMM::openVirtualPort: cannot be implemented in Windows MM MIDI
API!` — as a **warning**, not a thrown C++ exception. The Go wrapper
(`rtmididrv.Driver.OpenVirtualOut`/`OpenVirtualIn`) only turns a thrown
exception into a Go `error`, so on Windows it returns `err == nil` while
having created nothing. `internal/midiout.Open` and `internal/midiin.Open`
both trusted that return value to decide whether to fall back to `attach()`
by name — so on Windows the fallback never ran. The app logged
`MIDI out: "..." (virtual)` and reported normal operation, while every
byte written to that "virtual" port went nowhere.

The fix (`internal/midiout/midiout.go`, `internal/midiin/midiin.go`) is a
`runtime.GOOS != "windows"` guard around the `OpenVirtualOut`/`OpenVirtualIn`
attempt — skip it outright on Windows rather than trust its error return.
This is the actual reason MIDI-out (and any `NeedsMIDIIn` module's external
input) never worked on Windows before this fix, independent of the
isPush self-exclusion bug above; that bug was real too, but never got
exercised, since the virtual-port path always claimed false success first.

### Windows MIDI Services loopback endpoints — confirmed working

Confirmed live 2026-08-27, via Microsoft's `midi enum endpoints` /
`midi monitor` console tools (Windows MIDI Services SDK, Release
Candidate 4, GitHub Preview) on Windows on ARM. A loopback endpoint pair
created there — for example named `Push Tethered App (A)` /
`Push Tethered App (B)` — **is** visible to the legacy WinMM API this app
uses (`gm.GetOutPorts()`/`gm.GetInPorts()` via RtMidi), confirmed by
`cmd/pushapp -devices` listing both `Push Tethered App (A)` and `(B)`
alongside Push's own hardware ports. So Windows MIDI Services is a real
loopMIDI substitute, including on ARM64 where loopMIDI has no build.

A loopback pair has two ends: traffic sent into `(A)` arrives on `(B)`'s
monitor, not `(A)`'s own — this is not a bug, it is how a loopback pair
works. Point the app at one end (`-midi-out "Push Tethered App (A)"`) and
monitor or subscribe to the other end (`(B)`) to see or receive that
traffic.

## pushapp-ui

Wails v3 builds on Windows CI. The port picker appears when auto-detect
fails.

### Missing DLL errors at runtime

A Windows exe built with MSYS2/mingw dynamically links four DLLs by
default: `libgcc_s_seh-1.dll`, `libstdc++-6.dll`,
`libwinpthread-1.dll` (the mingw runtime), and `libusb-1.0.dll` (gousb's
`#cgo pkg-config: libusb-1.0`). On the build machine, these DLLs are on
PATH through MSYS2, so the build succeeds. If you copy only the `.exe`
to another Windows machine, it fails to launch with "missing DLL" errors
for all four.

Do not get these DLLs from another source (a random download, a
different MSYS2 install, a different mingw build) to work around a
missing-DLL error. A runtime DLL built against a different toolchain
version than the exe produces the error `0xc000007b`
(`STATUS_INVALID_IMAGE_FORMAT`) instead. The team confirmed this on
2026-08-18, against a VM report, after it manually sourced
`libusb-1.dll` and `libwinpthread-1.dll`. The fix is to make the exe not
need external copies of these DLLs at all.

The team applied this fix on 2026-08-18, in response to that VM report,
and confirmed it worked the same day against real Push 3 hardware (same
VM and USB passthrough):

- `libgcc_s_seh-1.dll`, `libstdc++-6.dll`, `libwinpthread-1.dll`: link
  these statically with `CGO_LDFLAGS="-static"`. A plain
  `-static-libgcc -static-libstdc++` misses `libwinpthread-1.dll`.
  MSYS2's toolchain ships static archives for all three, and this is
  safe by default. CI does this for `cmd/pushapp-ui`.
- `libusb-1.0.dll`: static linking needs `libusb-1.0.a` in MSYS2's
  `mingw64/lib`, and the team has not verified this on real hardware. A
  simpler, confirmed fix is to ship `mingw64/bin/libusb-1.0.dll` next to
  the exe, always from the same MSYS2 install that built the exe. CI
  copies it next to `cmd/pushapp-ui/bin/pushapp-ui.exe`.

Locally, in the MSYS2 shell before building:

```bash
export PATH="/mingw64/bin:$PATH"
export CGO_LDFLAGS="-static"
cd cmd/pushapp-ui && wails3 task build CGO_ENABLED=1
cp /mingw64/bin/libusb-1.0.dll bin/
```

Fixed 2026-09-02: CI now also packages the build as an NSIS installer
(`wails3 task package`). `cmd/pushapp-ui/build/windows/nsis/project.nsi`
bundles `libusb-1.0.dll` from `bin/` into the installer, next to the
app's own exe, so an installed copy needs none of this by-hand DLL
copying. The plain exe + DLL-next-to-it build above still runs, and CI
still produces it as a build step, but the release artifact users
download is the installer.

`windows-latest` does not ship NSIS (confirmed 2026-09-02: a search for
a preinstalled `makensis.exe` under both Program Files directories
found nothing). CI installs it with Chocolatey instead
(`choco install nsis`) before calling `wails3 task package`. That
package does not shim `makensis.exe` into `chocolatey\bin` — it
deploys straight to `C:\Program Files (x86)\NSIS`, which is the
directory CI adds to `$PATH`.

## Related

- [plans/2026-08-18-open-items.md](../../plans/2026-08-18-open-items.md) — remaining open items
- [architecture/module-host.md](../architecture/module-host.md) — MIDI out model
</content>
