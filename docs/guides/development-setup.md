# Development setup

**Status:** living guide
**Last verified:** 2026-08-18

Build natively on each target OS. Cross-compilation is not possible, because
of cgo (libusb + RtMidi).

## Requirements

**End users:** a single binary. No extra installs beyond the OS.

**Developers:**

- Go 1.25+
- A C toolchain (for cgo)
- libusb 1.0

Go downloads the `core/` module of
[`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack)
automatically. See [core/ dependency](#core-dependency).

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

Use a udev rule for Push 3 to get display access without root.

1. Create the file `/etc/udev/rules.d/99-push-display.rules` with this
   content:

```
SUBSYSTEM=="usb", ATTR{idVendor}=="2982", MODE="0666"
```

2. Run this command:

```bash
sudo udevadm control --reload-rules && sudo udevadm trigger
```

3. Replug the Push device.

For `pushapp-ui`, install `libgtk-4-dev` and `libwebkitgtk-6.0-dev`. See
[platform/linux.md](../platform/linux.md).

## Windows

Use the mingw-w64 toolchain (MSYS2) for cgo. Get libusb through MSYS2 or
vcpkg. MIDI uses WinMM, which is built in.

Display, USB, and MIDI work on Push 3 in a Windows 11 VM (confirmed
2026-08-18). See [platform/windows.md](../platform/windows.md).

## core/ dependency

`go.mod` and `cmd/pushapp-ui/go.mod` pin `core/` to a tagged version:

```
require github.com/federico-pepe/ableton-push-hack/core v0.2.0
```

Go gets this version from the module proxy. You do not need a local copy
of `ableton-push-hack`, and CI does not check it out.

To use a new `core/` change:

1. Merge the change in `ableton-push-hack`.
2. Push a new `core/vX.Y.Z` tag in that repository.
3. In this repository, run this command in the root and in
   `cmd/pushapp-ui`:

```bash
go get github.com/federico-pepe/ableton-push-hack/core@vX.Y.Z && go mod tidy
```

To test an unreleased `core/` change locally, add a temporary `replace`
line to your own `go.mod`. Do not commit this line.

## pushapp-ui (optional)

`cmd/pushapp-ui/` is a separate Go module.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 doctor
cd cmd/pushapp-ui && wails3 dev
```

This needs Node/npm. Details:
[cmd/pushapp-ui/README.md](../../cmd/pushapp-ui/README.md).

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
