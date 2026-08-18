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

## Related

- [guides/development-setup.md](../guides/development-setup.md)
- [open-questions.md](../open-questions.md) — Raspberry Pi untested
