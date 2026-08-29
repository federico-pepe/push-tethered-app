# Writing a process module (overview)

**Status:** living guide
**Last verified:** 2026-08-20
**Authoritative code:** [internal/host/procmod/](../../internal/host/procmod/)

A process module is any executable that the host spawns. The host talks to
it over newline-delimited JSON on stdin/stdout. This is the same behavior
contract as a Go module, over a different transport.

Full protocol: [architecture/process-modules.md](../architecture/process-modules.md).

## Quick start

```bash
go run ./cmd/pushapp -install examples/modules/hello-py
go run ./cmd/pushapp -module hello-py
```

## What you need

1. A directory with `manifest.json` and your script or executable.
2. A main loop that reads JSON lines from stdin.
3. Handlers for `init`, `handle`, `draw`, and `close`.
4. Responses on stdout. Write one JSON object per line, and flush it
   immediately.

## Lifecycle

```
host spawns child
  → init (device, theme, supported_ops) → {}
  → handle notifications (pad press, encoder, …) — no reply
  → draw request → {ops: [...], failed: 0}   [every frame]
  → close → {} then exit
```

## Draw ops

Build an array of ops that mirrors the Go types:

```json
{"kind": "rect", "params": {"x": 0, "y": 0, "w": 960, "h": 160, "c": {"R":0,"G":0,"B":0,"A":255}}}
{"kind": "text", "params": {"x": 8, "baseline": 80, "s": "hello", "c": {"R":255,"G":255,"B":255,"A":255}}}
```

Before you use an op, check `supported_ops` from `init`. The host might not
support every op.

### Colors

Every screen op's color parameter must trace back to a real
`core/push3.Palette` entry. See the color invariant in
[docs/architecture/design-system.md](../architecture/design-system.md). A
process module cannot import the Go `push3` package, so use
`cmd/genpalette` instead.

1. From the repository root, run `go run ./cmd/genpalette`.
2. This generates a `palette.json` file in every directory under
   `examples/modules/` that has a `manifest.json`. The file resolves all
   128 raw hardware indices to RGBA values from `core/push3.Palette`.
3. Copy `palette.json` into your own module's directory. `cmd/pushapp
   -install` copies every file in that directory, including
   `palette.json`, so the file travels with the module.
4. Load `palette.json` at startup:

```js
const PALETTE = JSON.parse(fs.readFileSync(path.join(__dirname, "palette.json"), "utf8"));
// PALETTE.byName["sky"]   -> {index, name, r, g, b, a} — the ~90 named colors
// PALETTE.byIndex[42]     -> same shape, any raw 0-127 hardware index
```

```python
with open(os.path.join(os.path.dirname(__file__), "palette.json")) as f:
    PALETTE = json.load(f)
# PALETTE["byName"]["sky"]  -> {"index", "name", "r", "g", "b", "a"}
# PALETTE["byIndex"][42]    -> same shape, any raw 0-127 hardware index
```

Both lookups return the same shape. `byIndex` pre-resolves every one of the
128 raw indices to its nearest defined entry. This gives the same
"nearest at or below" guarantee that `push3.ColorForIndex` gives on the Go
side, so a module never needs to reimplement that search. Rebuild
`palette.json` only when `core/push3.Palette` itself changes. This is rare,
because `core/push3.Palette` is a fixed, SysEx-sourced hardware table.
`palette.json` is a checked-in generated file. The build process does not
regenerate it on every run. See `examples/modules/hello-{js,py}` and
`beatcount-{js,py}` for working examples of both lookup styles, or the
`knobs-js` module (published separately, installable via
`-catalog-install knobs-js` — see
[catalog/schema.md](../../catalog/schema.md)).

## Host calls from child

Notifications carry no `id`:

```json
{"method": "set_pad", "params": {"note": 36, "colour": 11}}
```

Requests carry an `id` and expect a response:

```json
{"id": 1, "method": "store_get", "params": {}}
```

MIDI out (`send_cc`, `send_note`, `note_off`) requires `"needs_midi_out":
true` in the manifest. MIDI clock and transport out (`send_clock`,
`send_start`, `send_continue`, `send_stop`, all requests with no params)
use the same port and the same `needs_midi_out` flag.

External MIDI input is raw bytes from other software or hardware, not from
Push. It arrives as a `handle` notification with `"kind":
"external_midi"` and `"data": {"raw": "..."}`. The field `raw` is base64,
not a number array. Go's `encoding/json` encodes a `[]byte` field that way
by default, and this wire format mirrors the Go type directly instead of
reshaping it per language. Decode it with `base64.b64decode` in Python or
`Buffer.from(s, "base64")` in Node to get the actual MIDI bytes. For
example, a single `0xF8` clock tick decodes from `"+A=="`. External MIDI
input requires `"needs_midi_in": true` in the manifest. Unlike MIDI out, a
missing input port never blocks the module from loading. The module simply
never receives this event kind.

## Common mistakes

| Mistake | Symptom |
|---|---|
| Buffered stdout (Python) | Host hangs on first `draw` |
| Wrong colour type for pads | Use palette **index** for `set_pad`, RGBA for screen ops |
| Blocking on `handle` | Events pile up; host drops oldest |
| Using Image op | Not available over IPC |

## Language-specific guides

- [writing-a-python-module.md](writing-a-python-module.md)
- [writing-a-javascript-module.md](writing-a-javascript-module.md)

Examples index: [examples/modules/README.md](../../examples/modules/README.md).

## Writing a module in Go (or any compiled language)

The process-loader protocol doesn't care what spawned the child — a
compiled Go, Rust, or C binary that speaks JSON-over-stdio works exactly
like a Python or Node.js script. The one difference: a script runs
anywhere its interpreter is installed, but a compiled binary only runs on
the platform it was built for, and this repo's own cross-compilation
rule applies to a module's binary too (see the root `CLAUDE.md`'s
"Cross-platform builds" — build natively per target, do not cross-compile
cgo-free or not, to keep the guidance uniform across the project).

Use `exec_platforms` in `manifest.json` instead of `exec` to ship one
binary per target inside a single archive — see
[docs/architecture/process-modules.md](../architecture/process-modules.md#compiled-non-script-modules-exec_platforms)
for the manifest shape. A release workflow that builds each target
natively (e.g. one GitHub Actions matrix job per OS, same shape as this
repo's own `.github/workflows/build.yml`) and stitches the binaries plus
one shared `manifest.json` into a single `.tar.gz` release asset works
well.

## Publishing to the catalog

Once your module works with `-install`, users can find it without a
manual download: tag a release in your own repo with a `.tar.gz` of your
module directory attached as a release asset, then open a PR adding one
entry to this repo's `catalog/catalog.json`. See
[catalog/schema.md](../../catalog/schema.md) for the entry fields and the
publishing steps, and
[docs/architecture/process-modules.md](../architecture/process-modules.md#catalog-install)
for how `-catalog-install` resolves and downloads it.
