# Process-loaded modules

**Status:** implemented  
**Last verified:** 2026-08-18  
**Authoritative code:** [internal/host/procmod/](../../internal/host/procmod/), [internal/host/procinstall.go](../../internal/host/procinstall.go)

A module can be **any executable** — Python, Node.js, Rust, and more — that
speaks JSON-over-stdio. The host spawns it as a child process. `procmod.Proc`
implements `module.Module`, so the runtime treats it like an in-tree module.

Decision history: [plans/2026-08-17-process-loader.md](../../plans/2026-08-17-process-loader.md).

## Directory layout

```
~/.config/push-tethered-app/modules/<id>/
  manifest.json
  <executable and assets>
```

Installed via:

```bash
go run ./cmd/pushapp -install path/to/your-module      # a directory, or a .tar.gz/.tgz archive
go run ./cmd/pushapp -uninstall your-module-id
go run ./cmd/pushapp -list    # [installed] marker
go run ./cmd/pushapp -module your-module-id
```

`-install` accepts either a plain directory or a `.tar.gz`/`.tgz` archive of
one — `internal/host/procmod.InstallFromPath` (`internal/host/procmod/archive.go`)
detects which and extracts an archive to a temp directory first, via the
shared `internal/archiveutil` package (also used by `internal/catalog`
below). An archive whose contents are wrapped in a single top-level
directory (the shape `git archive` and GitHub's auto-generated source
tarballs produce) is unwrapped automatically —
`archiveutil.ResolveWrappedDir` looks for `manifest.json` at the root first,
then falls back to descending into a lone subdirectory.

## Catalog install

`internal/catalog` fetches a hosted `catalog.json` index
(`catalog/catalog.json` at this repo's root; schema documented in
[catalog/schema.md](../../catalog/schema.md)) of third-party modules, each
entry naming a GitHub repo and a release asset filename. There is no
central host and no checksum/signing — the catalog is an index of
pointers, and installing from it carries the same trust as installing any
other open-source release binary directly.

```bash
go run ./cmd/pushapp -catalog-list                  # id, name, description
go run ./cmd/pushapp -catalog-install <id>           # download latest release, install
go run ./cmd/pushapp -catalog-check-updates          # compare installed vs. catalog versions
go run ./cmd/pushapp -catalog-update <id>            # download latest release, replace installed files
go run ./cmd/pushapp -catalog-url <url> ...          # point at a different catalog.json, e.g. a fork
```

Flow for `-catalog-install`/`-catalog-update`: `catalog.Fetch` the index,
`catalog.Find` the entry by id, `catalog.ResolveAsset` calls GitHub's
`releases/latest` API to get the download URL and version tag for the
named asset, `catalog.DownloadAndExtract` downloads it (size-capped) and
extracts it the same way a local archive install does. `-catalog-install`
then calls `procmod.Install`; `-catalog-update` calls `procmod.Update`,
which replaces an existing installation's files (refusing if the new
manifest's `id` doesn't match) rather than refusing outright the way
`Install` does for a duplicate ID.

Update checking (`-catalog-check-updates`, and the "update available"
badge in `pushapp-ui`) compares an installed module's own
`manifest.json` `version` field against the catalog's resolved latest
release tag, using `catalog.CompareVersions` — a small local
`major.minor.patch` comparison matching this project's own version
scheme (see the root `CLAUDE.md`'s Releases section), not a general
semver library.

`pushapp-ui` exposes the same flow as `PushService.CatalogList`,
`CatalogInstall`, `CatalogUpdate`, and `CatalogCheckUpdates`
(`cmd/pushapp-ui/pushservice.go`), behind a "Browse catalog" button next
to the existing "Add module…" one.

## manifest.json

```json
{
  "id": "hello-py",
  "name": "Hello (Python example)",
  "version": "1.0.0",
  "author": "someone",
  "needs_midi_out": false,
  "exec": "python3 run.py"
}
```

`exec` is resolved relative to the module directory. On Windows, name the
full command the shell needs, for example `python.exe run.py`.

### Compiled (non-script) modules: `exec_platforms`

A module shipped as a compiled binary (Go, Rust, ...) rather than a
script needs a different binary per target, unlike a Python or Node.js
module where the same `exec` runs anywhere the interpreter is on PATH.
`exec_platforms` replaces `exec` for this case — a map keyed by
`"GOOS/GOARCH"` (`runtime.GOOS + "/" + runtime.GOARCH`, e.g.
`"darwin/arm64"`, `"linux/amd64"`, `"windows/amd64"`), each value an
`exec`-shaped command for that target:

```json
{
  "id": "my-go-module",
  "name": "My Go Module",
  "exec_platforms": {
    "darwin/arm64": "./bin/darwin-arm64/mymodule",
    "darwin/amd64": "./bin/darwin-amd64/mymodule",
    "linux/amd64": "./bin/linux-amd64/mymodule",
    "linux/arm64": "./bin/linux-arm64/mymodule",
    "windows/amd64": "./bin/windows-amd64/mymodule.exe"
  }
}
```

`Manifest.ResolvedExec()` (`internal/host/procmod/manifest.go`) picks the
entry matching the host `pushapp` is actually running on, and errors,
listing what's available, if none matches — a manifest can specify
`exec` **or** `exec_platforms`, not both meaningfully at once (only one
is read; `exec_platforms` wins if both are present). One archive, one
`asset_name`, bundles every platform's binary, so this needs no change
to the catalog schema — see [catalog/schema.md](../../catalog/schema.md).
`resolveExec`'s existing "resolve any token naming a real file in the
module directory to an absolute path" behaviour applies to whichever
command `ResolvedExec` returns, same as for a plain `exec`.

## Wire protocol

One JSON object per line on stdin (host→child) and stdout (child→host).
Messages must not contain literal newlines.

```json
{"id": 1, "method": "draw", "params": {}}
{"id": 1, "result": {"ops": [...], "failed": 0}}
```

| Field | Role |
|---|---|
| `id` | Request/response correlation; omit for notifications |
| `method` | Present on requests |
| `params` / `result` / `error` | Payload |

Not full JSON-RPC — no batching, no version field. Two peers, fixed method set.

### Host → child

| Method | Params | Response | Notes |
|---|---|---|---|
| `init` | `{device, theme, supported_ops}` | `{}` or error | Once, before anything else |
| `handle` | `{event}` | *(notification)* | Never blocks host |
| `draw` | `{}` | `{ops, failed}` | Must round-trip each frame |
| `close` | `{}` | `{}` or error | Child releases notes, then exits |

### Child → host

| Method | Params | Response |
|---|---|---|
| `set_pad` | `{note, colour}` | notification |
| `set_button` | `{cc, brightness}` | notification |
| `send_cc` | `{ch, cc, val}` | `{}` or error |
| `send_note` | `{ch, note, vel}` | `{}` or error |
| `note_off` | `{ch, note}` | `{}` or error |
| `log` | `{message}` | notification |
| `store_get` | `{}` | `{doc}` |
| `store_set` | `{doc}` | `{}` or error |

`send_*` requires `"needs_midi_out": true` in manifest.

## Draw ops

These ops use the same JSON shapes as the Go `internal/module` types.
Colour: `{"R":255,"G":255,"B":255,"A":255}` (Go `color.NRGBA` encoding).

**The image op is not available** over IPC. Raw pixels need an in-tree Go
module.

## Supervisor lifecycle

1. **Init:** spawn process, wire stdin/stdout, stderr → host log, send `init`
2. **Handle:** write notification, never wait
3. **Draw:** request/response with bounded timeout. On timeout: empty frame, logged
4. **Close:** send `close`, grace period, then kill if needed
5. **Crash:** reader detects closed pipe. Further calls no-op until re-activation

## Critical: flush stdout

The host reads **one line at a time**. Python buffers stdout when it is not
a TTY. **Flush after every line**, or the host appears hung. Node on POSIX
writes synchronously to pipes, but the same discipline applies everywhere.

## Language guides

- [writing-a-python-module.md](../guides/writing-a-python-module.md)
- [writing-a-javascript-module.md](../guides/writing-a-javascript-module.md)
- [writing-a-process-module.md](../guides/writing-a-process-module.md) — overview

Examples: [examples/modules/](../../examples/modules/).
