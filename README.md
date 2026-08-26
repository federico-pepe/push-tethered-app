# Push Tethered App

**Turn your Push 2 or Push 3 into a programmable surface.** It does not need a DAW.

This project is built on top of [Push Hack](https://github.com/federico-pepe/ableton-push-hack)

* 📕 [Read the Manual](MANUAL.md)
* 🛟 [Read how to contribute](CONTRIBUTING.md)
* 💬 [Join the Discord Community](https://discord.gg/8y6aYxy9nU) to discuss this project.

> [!WARNING]
> This project is <ins>**NOT**</ins> approved, endorsed or supported by Ableton. **Use at your own risk**.

## What this is

Push Tethered App is a desktop program. It takes full control of an
**Ableton Push 2 or Push 3 in tethered (controller) mode**. This includes
the screen, the 8×8 pad grid, encoders, buttons, and LEDs. The app does not
need Ableton Live.

The app is a **module host**. It runs small programs called **modules** —
each one draws on Push's screen and reacts to its controls. Built-in
modules include a control-surface monitor, a MIDI passthrough, a step
sequencer that can sync to an external MIDI clock, and a user-editable
remapper. You can write your own. See [MANUAL.md](MANUAL.md) for how to run
and configure the app itself.

## Why

Push is extraordinary hardware: a high-resolution display, a playable grid,
nine encoders, and dozens of buttons. In normal use, it is tightly coupled
to Live as its control surface. This project explores a different idea:
Push as an open platform for your own tools.

This can be a custom sequencer, a hardware monitor, a MIDI router to any
synth or DAW, or a visual performance instrument. It can also be an idea no
one has found yet. The goal is to make that practical on real hardware,
without reverse-engineering the device from scratch every time.

This repo handles **tethered Push on a desktop** (macOS, Linux, Windows).
It is a sibling of
[`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack),
which explores Push 3 in **standalone mode** over SSH. Both share the same
`core/` screen toolkit.

You can write modules in **Go**, **Python**, **JavaScript**, or any
language that can speak a small JSON protocol over stdin/stdout.

## How it works

1. **`pushapp`** claims Push's display over USB and reads control input
   through the operating system's MIDI stack.
2. **One module runs at a time.** Each frame, the module sends draw
   commands. The host renders them with a shared widget toolkit and pushes
   pixels to the screen at ~30 fps.
3. **Modules never touch USB or MIDI ports directly** — the host owns the
   hardware and exposes a simple API (set a pad color, send a CC, draw some
   text, and more).
4. **Optional extras:** modules can send MIDI out to other software, and
   can receive MIDI in from it (an external clock to sync to, for example).
   A desktop UI (`pushapp-ui`) lists and switches modules. You do not need
   the terminal. `pushapp-ui` can also pair and drive **several Push units
   at once** — each unit gets its own session and its own module,
   independently. `pushapp-ui` can also run alongside Ableton Live — see
   [MANUAL.md](MANUAL.md). Either binary can also mirror the screen live in
   a browser tab, for demos or debugging without the physical device — see
   MANUAL.md.

```
  You write a module          pushapp owns the hardware
  (Go / Python / JS / …)  →   USB display + OS MIDI in/out
         │                              │
         └──── draw ops, events ────────┘
```

## Try it

This needs a dev setup. See [docs/guides/development-setup.md](docs/guides/development-setup.md).

```bash
go run ./cmd/pushapp -list                              # built-in modules
go run ./cmd/pushapp                                    # run the first one
go run ./cmd/pushapp -install examples/modules/hello-py # install Python example
go run ./cmd/pushapp -module hello-py                   # run it
```

A desktop UI for switching modules:

```bash
cd cmd/pushapp-ui && wails3 dev
```

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

## Status

**Pre-alpha, but running.** We confirmed it on Push 2 and Push 3 hardware,
with the same binary. We confirmed process-loaded Python and Node.js
modules end-to-end.

End-user manual: **[MANUAL.md](MANUAL.md)** — pairing, MIDI port roles, running
alongside Live, troubleshooting.

Full developer documentation: **[docs/README.md](docs/README.md)** — protocol
reference, architecture, platform notes, and contributor guides. Open
questions live in [plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md).

## Related

- [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack) —
  standalone Push 3 research; source of the shared `core/` module.
- [`Ableton/push-interface`](https://github.com/Ableton/push-interface) —
  official Push 2 display and MIDI specification.
- [`ffont/push2-python`](https://github.com/ffont/push2-python) — working
  pyusb reference for Push 2.
</content>
</invoke>
