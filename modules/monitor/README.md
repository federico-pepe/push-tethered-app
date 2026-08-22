# monitor

The control-surface monitor: a live mirror of everything Push is sending —
pads, buttons, encoders, touch. It's the fastest way to see whether input
decoding, LED output, and the display are all working at once, and every
later module gets debugged by comparing against this one.

It also doubles as the reference implementation: no USB, no MIDI ports, no
goroutines, no locks — state is plain fields, since `Handle` and `Draw`
never run concurrently.

```bash
go run ./cmd/pushapp -module monitor
```
