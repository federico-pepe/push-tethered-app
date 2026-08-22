# beatcount

The smallest possible demo of `module.ExternalMIDI`: counts incoming MIDI
clock ticks and draws which beat of a 4/4 bar it's on across the pad grid,
as a digit.

It exists as a working reference for `NeedsMIDIIn` — the counterpart to
`thru`'s role for plain control-surface input — not as an instrument.
`seq` is the real consumer of external clock sync; this is the small
version of the same idea, easy to read start to finish in one sitting.

```bash
go run ./cmd/pushapp -module beatcount
```
