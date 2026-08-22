# seq

An 8-step, 8-lane gate sequencer. The pad grid *is* the sequencer: columns
are steps, rows are pitch lanes. Press a pad to toggle that lane's step on
or off; the playhead advances on its own and lights the current column,
sending a note for every active lane in it.

It exists to prove the parts of the module contract `monitor` and `thru`
don't exercise: MIDI driven by wall-clock timing rather than input, and
real persistence — the pattern and tempo survive a restart. It can also
follow an external MIDI clock instead of its own timing (`NeedsMIDIIn`)
while clock bytes are actively arriving.

Deliberately minimal: 8 steps (not 16), one fixed chromatic note per lane
(no scale or per-lane pitch), and a step's gate lasts until the next step
(no per-step gate length or velocity editing).

```bash
go run ./cmd/pushapp -module seq
```
