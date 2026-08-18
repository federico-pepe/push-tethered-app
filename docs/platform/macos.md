# macOS

**Status:** living reference  
**Last verified:** 2026-08-18  

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

## Probes

Swift tools in `tools/` (midimon, ledtest) — macOS only.

## Related

- [guides/development-setup.md](../guides/development-setup.md)
