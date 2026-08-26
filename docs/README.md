# Documentation

This directory has reference material and guides for **push-tethered-app**.
If you are new to the project, start with the root [README.md](../README.md).

## Reading paths

### I want to write a module

1. [guides/writing-a-go-module.md](guides/writing-a-go-module.md) — Go modules
   compiled into the binary
2. [guides/writing-a-process-module.md](guides/writing-a-process-module.md) —
   shared JSON-over-stdio protocol (any language)
3. [guides/writing-a-python-module.md](guides/writing-a-python-module.md) —
   walkthrough of `examples/modules/hello-py/`
4. [guides/writing-a-javascript-module.md](guides/writing-a-javascript-module.md)
   — walkthrough of `examples/modules/hello-js/`

Architecture background: [architecture/module-host.md](architecture/module-host.md),
[architecture/process-modules.md](architecture/process-modules.md).

### I want to draw something on the screen

[architecture/design-system.md](architecture/design-system.md) — the
shared widget catalog (knobs, faders, lists, meters, and more). It shows
how the `Frame` API maps to the catalog, and how to preview a screen with
`cmd/screensim` before you touch hardware.

### I want to build or contribute

1. [guides/development-setup.md](guides/development-setup.md) — per-OS toolchain
2. [guides/debugging.md](guides/debugging.md) — probes, capture, map verification
3. [protocol/usb-and-safety.md](protocol/usb-and-safety.md) — read before
   hardware sweeps
4. [architecture/stack-and-layout.md](architecture/stack-and-layout.md) —
   packages, stack choices, repo layout

Platform-specific notes: [platform/macos.md](platform/macos.md),
[platform/linux.md](platform/linux.md), [platform/windows.md](platform/windows.md).

### I want protocol / hardware facts

1. [protocol/display.md](protocol/display.md) — USB display transport
2. [protocol/midi-input.md](protocol/midi-input.md) — decoding pads, encoders, MPE
3. [protocol/led-output.md](protocol/led-output.md) — pad and button LEDs
4. [protocol/push2-vs-push3.md](protocol/push2-vs-push3.md) — device differences
5. [protocol/live-handshake.md](protocol/live-handshake.md) — raw SysEx traffic
6. [protocol/xport.md](protocol/xport.md) — xPort's own raw traffic (a
   read-only capture, seen when Live runs). The mechanism is not yet decoded.
6. [hardware-reference.md](hardware-reference.md) — upstream button map, palette,
   authoritative code pointers

Historical measurements and stack rationale:
[archive/feasibility.md](archive/feasibility.md) (frozen — do not edit).

What is still unresolved: [plans/2026-08-18-open-items.md](../plans/2026-08-18-open-items.md).

## Doc tiers

| Location | Purpose |
|---|---|
| [README.md](../README.md) | Project entry point — what, why, how |
| [MANUAL.md](../MANUAL.md) | End-user manual — how to run and configure the app |
| `docs/` (here) | Durable reference and how-to guides, for contributors |
| [plans/](../plans/) | Intent and decision history |
| [docs/archive/](archive/) | Frozen superseded writeups |
| [CLAUDE.md](../CLAUDE.md) | AI agent guidance + safety reminders |

**Doc sync rule:** If a change is meaningful to a future reader, update the
relevant doc in the same commit. This applies to new behavior, a new
protocol fact, or a resolved issue. Update README, MANUAL.md, or CLAUDE.md
too, if they apply. New behavior for the end user, not just an internal
API change, belongs in MANUAL.md, not only here.
