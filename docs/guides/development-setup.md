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
- A sibling checkout of
  [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack)
  for the `core/` module. See the `replace` directive in
  [go.mod](../../go.mod).

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

For `pushapp-ui`, install `webkit2gtk-4.1-dev`. See
[platform/linux.md](../platform/linux.md).

## Windows

Use the mingw-w64 toolchain (MSYS2) for cgo. Get libusb through MSYS2 or
vcpkg. MIDI uses WinMM, which is built in.

The display and USB path are still untested on real Windows hardware. MIDI
is tested. See [platform/windows.md](../platform/windows.md).

## core/ checkout

`go.mod` contains this line:

```
replace github.com/federico-pepe/ableton-push-hack/core => ../../Documents/GitHub/ableton-push-hack/core
```

1. Adjust the relative path to match your own layout, or clone
   ableton-push-hack as a sibling of this repository.

CI checks out `ableton-push-hack@main` and runs `go mod edit -replace`. This
step applies to CI only. The committed `go.mod` path stays unchanged.

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
