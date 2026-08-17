# Process loader — modules as any executable (phase 4)

**Status: DONE 2026-08-18.** Phase 4 of
[2026-08-17-module-host.md](2026-08-17-module-host.md). This is the phase that
actually delivers "anyone can write a module" — phases 0-3 only ever ran Go
code compiled into the binary. Verified end-to-end on Push 3 hardware: a real
pad press decoded by `internal/midi`, translated by the host, sent as a
`handle` notification over stdin to a **real spawned Node.js child process**,
which sent back a `set_pad` notification (lighting the physical LED) and then,
on the next `draw` request, the text describing which pad was pressed — all
rendered through the same `internal/host/render.go` pipeline every in-tree
module uses. Same round trip separately confirmed for the Python example.

## Context

Every module so far (`monitor`, `thru`, `seq`, `remap`) is an in-tree Go
package implementing `internal/module.Module` directly, in-process. That
proved the contract but not the promise: `internal/module`'s types
(`Event`, `Op`, `Meta`) already carry JSON tags specifically so a future
out-of-process module could reconstruct them — phase 1's own package doc says
so. This phase spends that investment: a module becomes **any executable**
that speaks a small JSON protocol over its own stdin/stdout.

Three decisions, confirmed 2026-08-17:

| Axis | Decision |
|---|---|
| Transport | stdio, newline-delimited JSON. No sockets, no named pipes, no per-OS branching. |
| Discovery | One directory per module: `manifest.json` + the module's own executable/assets. Not `core/hackcfg`'s `hack.json` — that's shaped for on-device sysvinit services this app doesn't have. |
| Image op over IPC | Dropped for v1. `image.NRGBA` doesn't serialise; out-of-process modules get every op except `Image`, documented as a known gap. A visualiser needing raw pixels stays an in-tree Go module. |

Neither `Install` nor `Uninstall` exist yet anywhere in the code — phases 1-3
only ever *mentioned* them in prose as "meaningful once modules live on disk."
This phase adds them for real to `Runtime`'s control API.

## The wire protocol

One JSON object per line, either direction, over the child's stdin (host→child)
and stdout (child→host). Framed by `\n`; a message must not contain a literal
newline (JSON without indentation never does).

```go
type Envelope struct {
    ID     int             `json:"id,omitempty"`     // present on a request that wants a response
    Method string          `json:"method,omitempty"` // present on a request
    Params json.RawMessage `json:"params,omitempty"`
    Result json.RawMessage `json:"result,omitempty"` // present on a successful response
    Error  string          `json:"error,omitempty"`  // present on a failed response
}
```

`ID` is how a response is matched to its request — set by whichever side
initiated the call, echoed back unchanged. A message with `Method` and no
`ID` is a **notification**: fire-and-forget, no response expected or sent.
This is deliberately not a full JSON-RPC implementation — no batching, no
`jsonrpc` version field — because the protocol only ever has two peers and a
fixed, small method set; adopting a general framework would be more surface
than this needs.

### Host → child

| Method | Params | Response | Notes |
|---|---|---|---|
| `init` | `{device, theme, supported_ops}` | `{}` or error | Sent once, before anything else. `device`/`theme` mirror `Host.Device()`/`Theme()`; `supported_ops` is `Host.SupportedOps()` so the child can degrade against an older host without a round trip per op. |
| `handle` | `{event: <module.Event JSON>}` | *(notification — no response)* | Matches the in-process contract's "never block in Handle": the host does not wait, so a slow child cannot stall the driver thread. Backpressure is the same bounded-queue-and-drop the in-process path already has, applied to the write side. |
| `draw` | `{}` | `{ops: [<module.Op>...], failed: N}` | The one call that must round-trip: the host needs this frame's ops before it can render. Bounded timeout (see Supervisor below); a timeout draws nothing for that frame and is logged, not fatal. |
| `close` | `{}` | `{}` or error | Gives the child a chance to call `note_off` for anything it's holding *before* acking, then the host closes stdin, waits for exit with a grace period, and kills it if it overstays. |

### Child → host

Mirrors `module.Host` 1:1 — no new behaviour, only JSON-shaping, same
principle as `cmd/pushapp-ui`'s `PushService`.

| Method | Params | Response |
|---|---|---|
| `set_pad` | `{note, colour}` | *(notification)* |
| `set_button` | `{cc, brightness}` | *(notification)* |
| `send_cc` | `{ch, cc, val}` | `{}` or error |
| `send_note` | `{ch, note, vel}` | `{}` or error |
| `note_off` | `{ch, note}` | `{}` or error |
| `log` | `{message}` | *(notification)* — the child formats its own string; `Log(format, args...)`'s printf-style doesn't translate cleanly across languages, so the wire form is just text. |
| `store_get` | `{}` | `{doc: <raw JSON or null>}` |
| `store_set` | `{doc: <raw JSON>}` | `{}` or error |

## Manifest format

```
~/.config/push-tethered-app/modules/<id>/
  manifest.json
  <the module's own executable and assets>
```

```json
{
  "id": "hello-py",
  "name": "Hello (Python example)",
  "version": "1.0.0",
  "author": "someone",
  "needs_midi_out": false,
  "exec": "./run.py"
}
```

`exec` is resolved relative to the manifest's own directory, so the directory
is fully self-contained and movable. On Windows this is whatever the shell
needs to run it (e.g. `python.exe run.py`) — the host does not add an
interpreter itself; the manifest names the exact command.

`Install(dirPath)` copies that directory into the config location (or, for a
directory already there, just registers it) and parses `manifest.json` into a
`module.Meta`. `Uninstall(id)` removes it and refuses if the module is
currently active (mirrors deleting a running program's files: unresolved,
resolved by requiring a switch away first).

## Supervisor lifecycle (`internal/host/procmod`)

`procmod.Proc` implements `module.Module`, so from `Runtime`'s point of view a
process-loaded module is indistinguishable from an in-tree one — `Activate`
does not need to know which kind it has.

- **`Init`** spawns the process (`os/exec`), wires stdin/stdout as the
  protocol transport, pipes the child's **stderr straight to the host's own
  log** (unparsed — that's the child's free-form debug output, not protocol),
  starts one reader goroutine that demultiplexes incoming lines by `ID`, then
  sends `init` and waits for its response with a timeout. A child that never
  answers `init` fails activation the same way a Go module's `Init` returning
  an error does.
- **`Handle`** marshals the event and writes the `handle` notification.
  Never blocks past a write to the pipe.
- **`Draw`** sends `draw`, blocks on that one response up to a bounded
  timeout, and unmarshals `ops` into the `*module.Frame` via `AppendRaw` —
  this is the second real caller of `AppendRaw`, exactly as phase 1's package
  doc predicted.
- **`Close`** sends `close`, waits for the response and process exit up to a
  grace period, then `Process.Kill()`s if it hasn't. A module that hangs on
  `Close` must not hang the host's own shutdown.
- A **crash mid-session** (pipe closed, process exited unexpectedly) is
  detected by the reader goroutine and surfaced as an error on any in-flight
  request; `Handle`/`Draw` calls after that point return/no-op with the crash
  logged rather than retrying a dead process. Recovery is the user
  re-`Activate`-ing, same as a Go module that returned an error from `Init`.

## Examples to ship

Two, in different languages with no shared code, proving the protocol rather
than proving Go:

- **Python** (`examples/modules/hello-py/`) — stdlib only, no dependencies to
  install. A single pad toggles a message; proves `handle`, `draw`, `set_pad`.
- **JS/Node** (`examples/modules/hello-js/`) — same behaviour, proves the
  protocol isn't Python-specific.

Deliberately not ports of `monitor` — small enough to read end-to-end as a
"how to write a module in a language that isn't Go" reference, which is the
actual point of this phase.

## What actually got built, and where it differs from the plan above

The wire protocol, manifest format and supervisor lifecycle shipped as
designed. Three things were discovered or added during implementation:

- **A real robustness bug, not just a test artifact: `call`/`notify` had no
  timeout on the *write* itself, only on waiting for the response.** Caught by
  a test that hung — a fake child in the test harness (an `io.Pipe`, so writes
  block synchronously with nothing draining them) exposed that `Close()`'s
  write of the `close` request could block forever if nothing was reading.
  The same failure mode applies to a real child with a full OS pipe buffer,
  just less likely to trigger. Fixed by moving the write itself into the same
  timeout-bounded `select`, in both `call` (request/response) and `notify`
  (fire-and-forget) — the latter matters most, since it's what backs `Handle`,
  and `Handle`'s "never blocks" guarantee was not actually true until this
  fix. `notifyTimeout` (200ms, matching `drawTimeout`) is the new constant.
- **`Runtime.Install`/`Uninstall`/`LoadInstalled` didn't exist anywhere before
  this phase** — phases 1-3 only ever mentioned them in prose. They're
  additive to the control API (`internal/host/procinstall.go`), splitting
  cleanly between a pure-filesystem half (`procmod.Install`/`Uninstall`/
  `ListInstalled` — no Runtime, no hardware, so a CLI flag works with no Push
  connected) and a Runtime half that adds the collision check against
  compiled-in modules and registers the result for immediate use. `-install
  <dir>` / `-uninstall <id>` flags on `cmd/pushapp` exercise this; `-list` now
  also shows installed modules, tagged `[installed]`.
- **`LoadInstalled` skips (and logs) an installed module whose ID collides
  with an existing one** rather than silently shadowing it — `findModule`
  checks compiled-in modules first, so an installed module with a colliding ID
  would otherwise be permanently unreachable with no indication why.
  `Install()` already refused this at install time; this is the same
  protection for the case where the installed-modules directory ends up with
  a collision some other way (hand-copied files, or a future built-in module
  shipping under an ID someone already used).

## Verification

- `internal/host/procmod`: 27 unit tests, including a hand-rolled fake child
  driven over real `io.Pipe`s (not a spawned process) so scenarios are
  deterministic and fast rather than timing-dependent: never responds to
  `init`, never responds to `draw` (proves the timeout), the process pipe
  closing mid-call (proves the *other* escape — done-channel, not just
  timeout), malformed JSON on one line, an unknown method from the child. All
  pass under `-race`.
- `internal/host`: `Install`/`Uninstall`/`LoadInstalled`/`findModule`/`List`
  tested directly against `&Runtime{}` literals (the existing pattern from
  `midiout_test.go`), with `procmod.SetInstalledRootForTest` redirecting the
  installed-modules root to a temp directory.
- Both example modules run for real on Push 3 hardware: `hello-py` confirmed
  via a captured frame showing "press a pad"; `hello-js` confirmed showing
  "pad 63  col 4 row 4" after an actual physical pad press — decoded by
  `internal/midi`, delivered to a real spawned Node child over stdin, answered
  with a `set_pad` notification that lit the real LED, then reflected in the
  next `draw` response and rendered to the real screen. Full round trip, no
  step faked.
- Not yet added: the plan's original idea of a `NeedsMIDIOut`-declared-but-
  refused test for a process-loaded module specifically — covered indirectly,
  since `Runtime.Activate`'s check already applies identically regardless of
  where `Meta.NeedsMIDIOut` came from (in-tree struct literal or parsed
  manifest field), and that check itself is already tested. No process-loaded
  example declares `needs_midi_out: true` yet to exercise it end-to-end.

## Docs to update in the same commit

- `CLAUDE.md` — the "Writing a module" section gains a pointer to this
  protocol for readers who don't want Go; the layout table gains
  `internal/host/procmod/`, `internal/host/procinstall.go` and
  `examples/modules/`; the phasing status line moves to phase 4 done.
- `docs/open-questions.md` — the "module manifest format... is undesigned"
  entry is resolved and removed.
