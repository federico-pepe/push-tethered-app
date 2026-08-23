# Writing a Python module

**Status:** living guide  
**Last verified:** 2026-08-20  
**Example:** [examples/modules/hello-py/](../../examples/modules/hello-py/)

Minimal process module — stdlib only, no pip install. Press a pad; the screen
shows which one and the pad lights green.

Shared protocol: [writing-a-process-module.md](writing-a-process-module.md).

## Install and run

```bash
go run ./cmd/pushapp -install examples/modules/hello-py
go run ./cmd/pushapp -module hello-py
```

Requires `python3` on PATH. Edit `manifest.json` `exec` if your system uses
a different interpreter name.

## manifest.json

```json
{
  "id": "hello-py",
  "name": "Hello (Python example)",
  "version": "1.0.0",
  "author": "push-tethered-app examples",
  "needs_midi_out": false,
  "exec": "python3 run.py"
}
```

Set `"needs_midi_out": true` only if you call `send_cc` / `send_note`.

## Structure of run.py

1. **Read stdin line by line** — each line is one JSON envelope
2. **Dispatch on `method`:** `init`, `handle`, `draw`, `close`
3. **Respond to requests** that include `id` (not notifications)
4. **Notify host** with `set_pad` / `set_button` (no `id`)

See [run.py](../../examples/modules/hello-py/run.py) for the full listing.

## Flush stdout — required

```python
def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()  # NOT optional
```

Python buffers stdout when connected to a pipe. Without flush, the host blocks
forever waiting for the first `draw` response.

## Pad events

`handle` sends:

```json
{"method": "handle", "params": {"kind": "pad", "data": {"note": 36, "pressed": true, "col": 0, "row": 0}}}
```

Light a pad (palette index, not RGB):

```python
notify("set_pad", {"note": note, "colour": 11})  # 11 = green
notify("set_pad", {"note": note, "colour": 0})   # off
```

Palette reference: [hardware-reference.md](../hardware-reference.md) → upstream
[push3-led-colors.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-led-colors.md).

## Draw response

```python
def draw(state):
    ops = [
        {"kind": "rect", "params": {"x": 0, "y": 0, "w": 960, "h": 160,
         "c": {"R": 0, "G": 0, "B": 0, "A": 255}}},
        {"kind": "text", "params": {"x": 8, "baseline": 80, "s": "press a pad",
         "c": {"R": 255, "G": 255, "B": 255, "A": 255}}},
    ]
    return {"ops": ops, "failed": 0}
```

Screen colours are RGBA dicts. Pad LED colours are separate palette indices.
Don't hand-copy RGB for a screen colour — load `palette.json`
(`cmd/genpalette`-generated, lives next to `run.py`) and look up by name
or by the same 0-127 index `set_pad` uses:
[writing-a-process-module.md](writing-a-process-module.md#colors).

## Close

Clear any lit pads before responding to `close`:

```python
elif method == "close":
    if state.lit_note is not None:
        notify("set_pad", {"note": state.lit_note, "colour": 0})
    respond(id_, {})
    break
```

## Debugging

- Run with `-module hello-py` and watch host stderr for protocol errors
- Child stderr is forwarded to host log unparsed — use for your own prints
- If nothing appears on screen, suspect missing flush first

## Next steps

- Add `store_get` / `store_set` for persistence
- Enable MIDI out in manifest and call `send_note`
- Set `"needs_midi_in": true` and handle `"kind": "external_midi"` to receive
  MIDI from other software (an external clock, for example) — see
  [examples/modules/beatcount-py/](../../examples/modules/beatcount-py/),
  and note `data.raw` there is **base64**, not a number array (Go's
  `encoding/json` encodes a `[]byte` field that way)
- Read [architecture/process-modules.md](../architecture/process-modules.md)
  for the full method table
