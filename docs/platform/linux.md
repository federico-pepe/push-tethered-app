# Linux

**Status:** living reference  
**Last verified:** 2026-08-18  

## Setup

```bash
sudo apt install libusb-1.0-0-dev libasound2-dev pkg-config build-essential
```

ALSA headers are required — `rtmididrv` needs `alsa/asoundlib.h`.

## USB display access

Without a udev rule, claiming interface 0 may require root. Push 3 vendor ID
example:

```
# /etc/udev/rules.d/99-push-display.rules
SUBSYSTEM=="usb", ATTR{idVendor}=="2982", MODE="0666"
```

Add Push 2's vendor ID if needed. Then:

```bash
sudo udevadm control --reload-rules && sudo udevadm trigger
```

Replug Push after adding the rule.

## MIDI

- Port names follow ALSA jack strings: **Live Port**, **User Port**, etc.
- Auto-detect works on measured setups
- MIDI out: host **creates** a virtual ALSA seq port

## pushapp-ui

Requires `webkit2gtk-4.1-dev` (CI installs this). This is the one place the
stack is not fully standalone.

## gousb busy

If claim fails with `LIBUSB_ERROR_BUSY`, detach interface 0 alone — never use
`SetAutoDetach(true)` (see [protocol/usb-and-safety.md](../protocol/usb-and-safety.md)).

## Raspberry Pi

Confirmed on a Pi 5 (Debian 13 "trixie", 64-bit arm64) 2026-08-18: `probe`,
`frametest` (29.9fps), and `pushapp -fps 30` (29.8fps, monitor module) all
verified against real Push 3 hardware. Same udev rule above applies — Pi OS
ships with no more permissive default than any other Linux. Built natively
via `.github/workflows/build.yml`'s `build-pi` job (`ubuntu-24.04-arm`, a real
aarch64 GitHub-hosted runner) rather than installing a Go toolchain on the Pi
itself; copy the resulting `pushapp`/`probe`/etc. binaries over and run them
directly — libusb/ALSA runtime libs are already present on stock Pi OS.

Pi 4 untested but expected identical (same rule, same arm64 target); see
[plans/2026-08-17-raspberry-pi-support.md](../../plans/2026-08-17-raspberry-pi-support.md)
for the original unknowns list, mostly closed by this test.

## Related

- [guides/development-setup.md](../guides/development-setup.md)
- [open-questions.md](../open-questions.md)
