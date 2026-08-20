# macOS

**Status:** living reference  
**Last verified:** 2026-08-20  

## Setup

```bash
brew install libusb
export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig:$PKG_CONFIG_PATH
```

Intel Macs may use `/usr/local/lib/pkgconfig` instead of Homebrew's Apple Silicon
path.

## MIDI

- Push ports appear as CoreMIDI **Live Port**, **User Port**, **External Port**
  (Push 3) — auto-detect works
- MIDI out: host **creates** a virtual port via CoreMIDI — no user setup

## Display

USB display claim on interface 0 confirmed on Push 2 and Push 3.

## pushapp-ui

Wails v3 dev/build works with standard Xcode CLI tools + Node/npm. See
[cmd/pushapp-ui/README.md](../../cmd/pushapp-ui/README.md).

### Missing libusb dylib on end-user Macs

`gousb` links `libusb` dynamically, and a build linked against Homebrew's
formula points at an absolute path — `/opt/homebrew/opt/libusb/lib/libusb-1.0.0.dylib`
on Apple Silicon (`/usr/local/opt/...` on Intel). That path only exists on
machines with `brew install libusb` done. Copying the `.app` to a Mac
without Homebrew's libusb fails to launch with a dyld "Library not loaded"
error — same class of bug as Windows' missing-DLL issue above, RtMidi is
unaffected since it only links Apple system frameworks (CoreMIDI/CoreAudio).

Fix: `cmd/pushapp-ui/build/darwin/Taskfile.yml`'s `create:app:bundle` task
runs `bundle:libusb` before codesigning — it copies
`libusb-1.0.0.dylib` into `Contents/Frameworks/` and rewrites the binary's
load path with `install_name_tool` to `@executable_path/../Frameworks/libusb-1.0.0.dylib`.
Applies to `task darwin:package`/`package:universal`/`package:dmg` (the
distributable bundle); `task darwin:run`'s dev `.dev.app` is left pointing
at the Homebrew dylib since dev machines already have it installed.

Docker cross-compiled bundles (built from a Linux/Windows host) skip this
step (`platforms: [darwin]`) and still need manual signing/dylib-fixing on
a real Mac before distribution — same caveat `codesign:skip` already prints.

## Probes

Swift tools in `tools/` (midimon, ledtest) — macOS only.

## Related

- [guides/development-setup.md](../guides/development-setup.md)
