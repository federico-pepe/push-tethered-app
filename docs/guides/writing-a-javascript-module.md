# Writing a JavaScript module

**Status:** living guide  
**Last verified:** 2026-08-20  
**Example:** [examples/modules/hello-js/](../../examples/modules/hello-js/)

Same behaviour as the Python example — press a pad, screen shows coordinates,
pad lights green. Proves the protocol is not Python-specific.

Shared protocol: [writing-a-process-module.md](writing-a-process-module.md).

## Install and run

```bash
go run ./cmd/pushapp -install examples/modules/hello-js
go run ./cmd/pushapp -module hello-js
```

Requires Node.js. `manifest.json`:

```json
{
  "id": "hello-js",
  "name": "Hello (Node.js example)",
  "exec": "node run.js"
}
```

## Structure of run.js

Uses `readline` on stdin (non-terminal mode):

```javascript
const rl = readline.createInterface({ input: process.stdin, terminal: false });

rl.on("line", (line) => {
  const env = JSON.parse(line);
  switch (env.method) {
    case "init": respond(env.id, {}); break;
    case "handle": /* ... */ break;
    case "draw": respond(env.id, draw()); break;
    case "close": /* clear pads */ rl.close(); process.exit(0);
  }
});
```

See [run.js](../../examples/modules/hello-js/run.js) for the full listing.

## stdout discipline

```javascript
function send(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}
```

Node writes to pipe stdout synchronously on POSIX — no Python-style flush trap.
Still write **one complete JSON line per call**; do not batch or buffer manually.

On Windows or embedded runtimes, verify the same immediate-write behaviour.

## Pad handling

Same as Python — `handle` with `kind: "pad"`, notify host:

```javascript
notify("set_pad", { note, colour: 11 });  // green palette index
notify("set_pad", { note, colour: 0 });   // off
```

## Draw ops

Identical JSON shapes to Python and Go:

```javascript
{ kind: "text", params: { x: 8, baseline: 80, s: "hello",
  c: { R: 255, G: 255, B: 255, A: 255 } } }
```

Don't hand-copy RGB for a screen colour — load `palette.json`
(`cmd/genpalette`-generated, lives next to `run.js`) and look up by name
or by the same 0-127 index `set_pad` uses:
[writing-a-process-module.md](writing-a-process-module.md#colors).

## Close

Release lit pads before acking `close`, then exit cleanly so the supervisor
does not kill the process.

## Comparison with Python

| Topic | Python | Node.js |
|---|---|---|
| Stdin loop | `for line in sys.stdin` | `readline` interface |
| Flush | **Required** (`sys.stdout.flush()`) | Usually automatic on pipes |
| Dependencies | stdlib only | `node:readline` built-in |

## Next steps

- [writing-a-python-module.md](writing-a-python-module.md) — persistence and
  MIDI out patterns (same wire format)
- Set `"needs_midi_in": true` and handle `"kind": "external_midi"` to receive
  MIDI from other software (an external clock, for example) — see
  [examples/modules/beatcount-js/](../../examples/modules/beatcount-js/),
  and note `data.raw` there is **base64**, not a number array (Go's
  `encoding/json` encodes a `[]byte` field that way) — `Buffer.from(s,
  "base64")` decodes it
- [architecture/process-modules.md](../architecture/process-modules.md)
