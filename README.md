# Push Tethered App

**Turn your Push 2 or Push 3 into a programmable controller without Ableton Live.**

This project is built on top of [Push Hack](https://github.com/federico-pepe/ableton-push-hack)

* 📕 [Read the Manual](MANUAL.md)
* 🛟 [Read how to contribute](CONTRIBUTING.md)
* 💬 [Join the Discord Community](https://discord.gg/8y6aYxy9nU) to discuss this project.

> [!WARNING]
> This project is <ins>**NOT**</ins> approved, endorsed or supported by Ableton. **Use at your own risk**.

## What this is

**Push Tethered App** is a cross-platform desktop app. It takes full control of an **Ableton Push 2 or Push 3 in tethered mode**: the screen, the 8×8 pad grid, encoders, buttons, and LEDs. You can use your Push without Ableton Live.

This app is a **module host**: this means that it can runs small programs called **modules** that anyone can develop. Each module can draws on Push's screen and reacts to its controls. There are some built-in modules to showcase what is possible but you can also write your own module. See [MANUAL.md](MANUAL.md) for how to run and configure the app.

## Why

Push is extraordinary hardware: a high-resolution display, a playable grid, encoders, and a lot of buttons. Normal use ties it tightly to Live as a control surface.

This project opens Push as a platform for your own tools instead: a custom sequencer, a MIDI router to any synth or DAW, a performance instrument, or an idea no one has built yet. The goal is to make this practical on real hardware, without a reverse-engineering effort each time.

This repo covers **tethered Push on a desktop** (macOS, Linux, Windows). It is a sibling of [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack), which covers Push 3 in **standalone mode** over SSH. Both projects share the same `core/` toolkit.

You can write modules in **Go**, **Python**, **JavaScript**, or any language that can speak a small JSON protocol over stdin/stdout.

For how the host and modules work together, see
[docs/architecture/module-host.md](docs/architecture/module-host.md). For
running `pushapp-ui`, several Push units at once, alongside Live, or
mirroring the screen in a browser, see [MANUAL.md](MANUAL.md).

## Get it

Download the latest build from
[GitHub Releases](https://github.com/federico-pepe/push-tethered-app/releases).
See [MANUAL.md](MANUAL.md) for setup.

To build from source instead, see
[docs/guides/development-setup.md](docs/guides/development-setup.md).

## Write a module

Pick a language:

| Language | Guide | Example |
|---|---|---|
| Go (compiled in) | [writing-a-go-module.md](docs/guides/writing-a-go-module.md) | `modules/monitor/` |
| Python | [writing-a-python-module.md](docs/guides/writing-a-python-module.md) | `examples/modules/hello-py/` |
| JavaScript (Node) | [writing-a-javascript-module.md](docs/guides/writing-a-javascript-module.md) | `examples/modules/hello-js/` |

All out-of-process modules share the same wire protocol. If you want the
overview first, start with
[writing-a-process-module.md](docs/guides/writing-a-process-module.md).

## Reference

- **[MANUAL.md](MANUAL.md)** — end-user manual: pairing, MIDI port roles,
  running alongside Live, troubleshooting
- **[docs/README.md](docs/README.md)** — full developer documentation
  index: protocol reference, architecture, platform notes, contributor
  guides
- **[plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md)** —
  open questions
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — how to contribute

Other projects:

- [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack) —
  standalone Push 3 research; source of the shared `core/` module
- [`Ableton/push-interface`](https://github.com/Ableton/push-interface) —
  official Push 2 display and MIDI specification
- [`ffont/push2-python`](https://github.com/ffont/push2-python) — working
  pyusb reference for Push 2