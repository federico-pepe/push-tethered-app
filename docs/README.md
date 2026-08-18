# Documentation

Reference and guides for **push-tethered-app**. Start from the root
[README.md](../README.md) if you are new to the project.

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
5. [hardware-reference.md](hardware-reference.md) — upstream button map, palette,
   authoritative code pointers

Historical measurements and stack rationale:
[archive/feasibility.md](archive/feasibility.md) (frozen — do not edit).

What's still unresolved: [plans/2026-08-18-open-items.md](../plans/2026-08-18-open-items.md).

## Doc tiers

| Location | Purpose |
|---|---|
| [README.md](../README.md) | Project entry point — what, why, how |
| `docs/` (here) | Durable reference and how-to guides |
| [plans/](../plans/) | Intent and decision history |
| [docs/archive/](archive/) | Frozen superseded writeups |
| [CLAUDE.md](../CLAUDE.md) | AI agent guidance + safety reminders |

**Doc sync rule:** when a change is meaningful to a future reader — new
behaviour, a protocol fact, a resolved issue — update the relevant doc here
(and README or CLAUDE.md if appropriate) in the same commit.
