# Development setup

**Status:** living guide  
**Last verified:** 2026-08-18  

Build **natively on each target OS** — no cross-compilation (cgo: libusb +
RtMidi).

## Requirements

**End users:** single binary, no extra installs beyond the OS.

**Developers:**

- Go 1.25+
- C toolchain (for cgo)
- libusb 1.0
- Sibling checkout of
  [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack)
  for the `core/` module — see `replace` in [go.mod](../../go.mod)

## macOS

```bash
brew install libusb
export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig:$PKG_CONFIG_PATH
go build ./...
go test ./...
```

## Linux

```bash
sudo apt install libusb-1.0-0-dev libasound2-dev pkg-config build-essential
```

Display access without root — udev rule for Push 3:

```
# /etc/udev/rules.d/99-push-display.rules
SUBSYSTEM=="usb", ATTR{idVendor}=="2982", MODE="0666"
```

```bash
sudo udevadm control --reload-rules && sudo udevadm trigger
# replug Push
```

For `pushapp-ui`: `webkit2gtk-4.1-dev`. See [platform/linux.md](../platform/linux.md).

## Windows

mingw-w64 toolchain (MSYS2) for cgo; libusb via MSYS2 or vcpkg. MIDI uses
WinMM (built in).

**Display/USB path still untested on real Windows hardware.** MIDI has been
tested — see [platform/windows.md](../platform/windows.md).

## core/ checkout

`go.mod` contains:

```
replace github.com/federico-pepe/ableton-push-hack/core => ../../Documents/GitHub/ableton-push-hack/core
```

Adjust the relative path to match your layout, or clone ableton-push-hack as a
sibling of this repo.

CI checks out `ableton-push-hack@main` and runs `go mod edit -replace` — that
is CI-only; the committed `go.mod` path is unchanged.

## pushapp-ui (optional)

Separate Go module under `cmd/pushapp-ui/`:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 doctor
cd cmd/pushapp-ui && wails3 dev
```

Needs Node/npm. Details: [cmd/pushapp-ui/README.md](../../cmd/pushapp-ui/README.md).

## Verify build

```bash
go build ./... && go vet ./... && go test ./...
```

## Common commands

```bash
go run ./cmd/pushapp -list
go run ./cmd/pushapp -module monitor
go run ./cmd/probe              # USB descriptors, read-only
go run ./cmd/frametest          # display probe
go run ./cmd/midiouttest -list  # MIDI out ports
```

Flags: `-fps`, `-module`, `-no-display`, `-no-leds`, `-midi-out`, `-capture`,
`-install`, `-uninstall`.

## Related

- [platform/macos.md](../platform/macos.md)
- [platform/linux.md](../platform/linux.md)
- [platform/windows.md](../platform/windows.md)
- [architecture/stack-and-layout.md](../architecture/stack-and-layout.md)
