# macOS

**Status:** living reference  
**Last verified:** 2026-08-20  

## Setup

```bash
brew install libusb
export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig:$PKG_CONFIG_PATH
```

Intel Macs can use `/usr/local/lib/pkgconfig` instead of Homebrew's Apple
Silicon path.

## MIDI

- Push ports appear as CoreMIDI **Live Port**, **User Port**, and
  **External Port** (Push 3). Auto-detect works.
- MIDI out: the host **creates** a virtual port with CoreMIDI. The user
  does not need to set up anything.

## Display

The team confirmed the USB display claim on interface 0 on Push 2 and
Push 3.

## Live's background helper (confirmed 2026-08-20)

Live does not claim interface 0 itself. A background helper does this
instead: `<Live.app>/Contents/Helpers/Push3.app/Contents/MacOS/Push3`
(bundle id `com.ableton.Push3`) for Push 3, or `Push2DisplayProcess.app`
under `<Live.app>/Contents/Push2/` for Push 2. `launchd` does not manage
this helper. Live spawns it as a plain child process at launch, with
`--parent-process-id=<Live's pid>`. For the full ownership matrix,
timings, and doc corrections, see
[protocol/usb-and-safety.md](../protocol/usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20).

Commands used to identify it:

```bash
ps -Ao pid,ppid,user,command | grep -iE 'ableton|push'
launchctl list | grep -iE 'ableton|push'          # empty — confirms not launchd-managed
ls -la ~/Library/LaunchAgents /Library/LaunchAgents /Library/LaunchDaemons | grep -iE 'ableton|push'
find "/Applications/Ableton Live 12 Suite.app/Contents" -maxdepth 2 -iname "*push*"
```

## pushapp-ui

Wails v3 development and builds work with standard Xcode command line
tools and Node/npm. See
[cmd/pushapp-ui/README.md](../../cmd/pushapp-ui/README.md).

### "Damaged or incomplete" at launch (CFBundleExecutable mismatch)

Fixed 2026-09-02. `build/darwin/Info.plist` and `Info.dev.plist` had
`CFBundleExecutable` set to `Push Tethered App` (`BUNDLE_NAME`, the
`.app`'s display name), but `create:app:bundle` copies the built binary
into `Contents/MacOS/` under its own name, `pushapp-ui` (`APP_NAME`).
macOS looks for `Contents/MacOS/<CFBundleExecutable>` to launch the
app; that file never existed, so every `.app` this project ever
packaged was structurally broken.

This went unnoticed because CI only ran `wails3 build` (a bare binary,
no `.app`) until the v0.2.1-alpha release, which was the first to run
`darwin:package:dmg` and hit it live: the ad-hoc `codesign -dv` output
showed `Format=bundle` (not `app bundle`) and
`Executable=.../Contents/Info.plist`, both signs that codesign could
not find the binary it was told to look for. macOS shows this exact
mismatch to the user as "You can't open the application ... because it
may be damaged or incomplete" — a LaunchServices bundle-validation
error, not a Gatekeeper trust/quarantine warning (that one reads
"Apple could not verify..." with an Open Anyway option instead).

Fix: `CFBundleExecutable` is now `pushapp-ui` in both plists, matching
the file `create:app:bundle` actually places in `Contents/MacOS/`.
Confirmed 2026-09-02 by patching a downloaded v0.2.1-alpha `.app`'s
`Info.plist` and re-signing: `codesign -dv` then reported
`Format=app bundle with Mach-O thin (arm64)` and
`Executable=.../Contents/MacOS/pushapp-ui`, and `open` launched it.

### Missing libusb dylib on end-user Macs

`gousb` links `libusb` dynamically. A build linked against Homebrew's
formula points at an absolute path:
`/opt/homebrew/opt/libusb/lib/libusb-1.0.0.dylib` on Apple Silicon
(`/usr/local/opt/...` on Intel). That path exists only on machines where
you ran `brew install libusb`. If you copy the `.app` to a Mac without
Homebrew's libusb, it fails to launch with a dyld "Library not loaded"
error. This is the same class of bug as Windows' missing-DLL issue.
RtMidi is not affected, because it links only Apple system frameworks
(CoreMIDI, CoreAudio).

Fix: `cmd/pushapp-ui/build/darwin/Taskfile.yml`'s `create:app:bundle`
task runs `bundle:libusb` before it signs the code. This task copies
`libusb-1.0.0.dylib` into `Contents/Frameworks/` and rewrites the
binary's load path with `install_name_tool`, to
`@executable_path/../Frameworks/libusb-1.0.0.dylib`. This applies to
`task darwin:package`, `package:universal`, and `package:dmg` (the
distributable bundle). `task darwin:run`'s dev `.dev.app` still points
at the Homebrew dylib, because dev machines already have it installed.

Docker cross-compiled bundles (built from a Linux or Windows host) skip
this step (`platforms: [darwin]`). These bundles still need manual
signing and dylib fixing on a real Mac before distribution.
`codesign:skip` already prints this same caution.

Fixed 2026-09-02: `.github/workflows/build.yml` now runs
`wails3 task darwin:package:dmg` for the macOS job, not `wails3 build`. This
gives the release a real `.app` bundle in a `.dmg`, with the
`bundle:libusb` step above and ad hoc code signing already applied, so
the downloaded `.dmg` needs no separate libusb install.

## Probes

The Swift tools in `tools/` (midimon, ledtest) work on macOS only.

## Related

- [guides/development-setup.md](../guides/development-setup.md)
</content>
