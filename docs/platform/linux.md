# Linux

**Status:** living reference  
**Last verified:** 2026-08-19  

## Setup

```bash
sudo apt install libusb-1.0-0-dev libasound2-dev pkg-config build-essential
```

You must install the ALSA headers, because `rtmididrv` needs `alsa/asoundlib.h`.

## USB display access

If you do not add a udev rule, claiming interface 0 can require root access.
This is the Push 3 vendor ID example:

```
# /etc/udev/rules.d/99-push-display.rules
SUBSYSTEM=="usb", ATTR{idVendor}=="2982", MODE="0666"
```

If you need Push 2 support, add its vendor ID too. Then run:

```bash
sudo udevadm control --reload-rules && sudo udevadm trigger
```

After you add the rule, disconnect and reconnect Push.

## MIDI

- Auto-detect works on measured setups.
- MIDI out: the host **creates** a virtual ALSA seq port.

Port names carry role strings. `rtmididrv` also appends an ALSA
client:port address to every name. This is not the clean
`"... Live Port"` string that a naive suffix match expects. The team
measured this live on real Raspberry Pi 5 hardware on 2026-08-19, with a
Push 2:

```
Ableton Push 2:Ableton Push 2 Live Port 28:0
Ableton Push 2:Ableton Push 2 User Port 28:1
```

If the code does not strip the trailing `" 28:0"`/`" 28:1"` first, neither
name ends in `"Live Port"`/`"User Port"` any more. As a result, role
detection missed both cables. Each cable became its own fake
single-cable "unit", and the code marked both of them as Live by
mistake. `internal/midi.unitKeyOf` strips this suffix (`alsaClientPort`)
before it classifies a name. This bug never broke pairing itself. ALSA
gives the in and out sides of one cable the identical string, so
exact-name matching still found it. Only the role and unit grouping
failed.

## pushapp-ui

`pushapp-ui` needs `webkit2gtk-4.1-dev` (CI installs this package). This
is the one place where the stack is not fully standalone.

## gousb busy

If claim fails with `LIBUSB_ERROR_BUSY`, detach interface 0 alone. Never
use `SetAutoDetach(true)`. See
[protocol/usb-and-safety.md](../protocol/usb-and-safety.md).

## Raspberry Pi

The team confirmed this on a Pi 5 (Debian 13 "trixie", 64-bit arm64) on
2026-08-18: `probe`, `frametest` (29.9fps), and `pushapp -fps 30`
(29.8fps, monitor module) all ran correctly against real Push 3
hardware. The same udev rule above applies. Pi OS does not ship with a
more permissive default than any other Linux distribution. The build
uses `.github/workflows/build.yml`'s `build` job's `ubuntu-24.04-arm`
matrix entry (a real aarch64 GitHub-hosted runner, folded into the main
matrix 2026-08-27 — no longer a separate `build-pi` job), and builds the
binary natively there, instead of installing a Go toolchain on the Pi
itself. Copy the resulting `pushapp` binary to the Pi and run it directly
(`probe`/`frametest`/`mapcheck` are diagnosis tools, not part of this build —
get those from `diagnostics.yml` instead if a Pi needs one). Stock Pi OS
already has the libusb and ALSA runtime libraries. That same matrix entry
now also builds `cmd/pushapp-ui` (uploaded as `pushapp-ui-ubuntu-24.04-arm`)
— compiled only, not yet run on real Pi hardware.

The team assumes that Pi 4 works the same way, because it uses the same
rule and the same arm64 target, and has not tested Pi 4 separately. If
Pi 4 does not work, real users will find this instead of a dedicated
test pass. See
[plans/2026-08-17-raspberry-pi-support.md](../../plans/2026-08-17-raspberry-pi-support.md)
for the original list of unknowns. This test closed most of them.

The team confirmed this again on 2026-08-19, on the same Pi 5, with a
Push 2 and the ALSA port-naming fix above. `pushapp -devices` grouped
its two cables correctly into one unit, with the right roles.
`pushapp -module monitor` claimed the display and rendered correctly:
916 frames at 29.9fps over 30 seconds, a clean SIGINT shutdown, and
cleared LEDs. The team built the binaries with
[`.github/workflows/diagnostics.yml`](../../.github/workflows/diagnostics.yml)
(`workflow_dispatch`) and copied them over with `scp`. The Pi itself
needs no Go toolchain and no libusb or ALSA development headers, because
it only runs the binary and does not build it.

## Related

- [guides/development-setup.md](../guides/development-setup.md)
- [plans/2026-08-18-open-items.md](../../plans/2026-08-18-open-items.md)
</content>
