# CI coverage for cmd/pushapp-ui

**Status: DONE 2026-08-18.** Follow-up from phase 3
([2026-08-17-module-host.md](2026-08-17-module-host.md)), captured while
answering "would this build if merged to main?" `.github/workflows/build.yml`
now runs `wails3 build` for `cmd/pushapp-ui` on all three OSes in the same
matrix job as the CLI build, using the chosen middle ground below (full
`wails3 build`, not the manual `npm run build` + `go build` route) since the
frontend needs generated bindings either way and `wails3 build` produces those
itself. Not yet confirmed green on a real CI run — the workflow file is
written and locally sanity-checked (YAML parses, the same commands run
successfully on this machine) but hasn't executed on GitHub-hosted runners
yet.

## Context

`cmd/pushapp-ui` (the Wails v3 module switcher) is deliberately a separate Go
module from the rest of the repo — see phase 3's notes in the module-host
plan for why. One direct consequence: `.github/workflows/build.yml` never
touches it. The workflow builds four named binaries (`probe`, `frametest`,
`mapcheck`, `pushapp`) and runs `go vet ./...` / `go test ./...` at repo root,
and since `cmd/pushapp-ui` has its own `go.mod`, it is invisible to that root
`./...` — confirmed via `go list ./...` at repo root not mentioning it.

**This means merging `module-host` to `main` is safe — CI stays green,
unaffected — but it also means CI is not building or testing the UI at all
today.** Nothing currently catches a `cmd/pushapp-ui` regression before it
reaches `main`.

## What's needed

A new job (or new steps in the existing matrix, scoped to skip Windows/Linux
if the UI is meant to start macOS-only) that:

1. **Installs Node/npm.** Present by default on GitHub-hosted runners, but
   worth pinning a version explicitly (`actions/setup-node`) rather than
   trusting whatever the runner image ships.
2. **Installs the `wails3` CLI**: `go install
   github.com/wailsapp/wails/v3/cmd/wails3@latest`. Resolves through the Go
   module proxy, not GitHub releases directly, so it isn't exposed to a
   GitHub outage the way `wails3`'s own installer script might be.
3. **Linux needs `webkit2gtk-4.1-dev` added to the apt install line.** Not
   currently there — the existing Linux step only installs
   `libusb-1.0-0-dev libasound2-dev pkg-config`.
4. **The real snag: `cmd/pushapp-ui/go.mod`'s replace for
   `ableton-push-hack/core` is a hardcoded relative path** (four `../` levels,
   reaching `~/Documents/GitHub/ableton-push-hack/core` on Federico's machine
   specifically) that will not resolve on a CI runner.

   The root workflow already solves the identical problem for its own
   dependency: it checks out `ableton-push-hack@main` into a fixed
   subdirectory of its own workspace (`.ableton-push-hack`), then runs
   `go mod edit -replace github.com/federico-pepe/ableton-push-hack/core=./.ableton-push-hack/core`
   — CI-only, never touching the committed `go.mod`.

   A UI build job needs the same trick run **inside `cmd/pushapp-ui`**,
   pointed at that same checkout, e.g.:

   ```bash
   go mod edit -replace github.com/federico-pepe/ableton-push-hack/core=../../.ableton-push-hack/core
   ```

   (relative to `cmd/pushapp-ui`, assuming the checkout step still lands
   `.ableton-push-hack` at the repo root as it does today). The other replace
   in that `go.mod` — `github.com/federico-pepe/push-tethered-app => ../..` —
   does **not** need editing for CI: unlike the core/ dependency, it points at
   a sibling directory *within the same repo checkout*, so `../..` from
   `cmd/pushapp-ui` always resolves to the repo root regardless of how the CI
   workspace nests things.
5. **Decide what "build" means for CI purposes.** Options, cheapest first:
   - `go build .` / `go vet .` inside `cmd/pushapp-ui` (compiles the Go side
     only; does not need `frontend/dist` to exist unless the `//go:embed`
     directive is satisfied — it is not, without an `npm run build` first, so
     this alone will fail on the embed just like it did locally before the
     frontend was built).
   - `npm ci && npm run build` in `frontend/`, then `go build .` — the
     minimum that actually produces a working binary, mirroring what
     `wails3 build` does under the hood without needing the CLI at all.
   - Full `wails3 build` — closest to what a human would run locally, exercises
     icon generation and binding generation too, but is the most moving parts
     and the slowest.
6. **Platform scope.** Wails' vanilla template already ships `build/android`
   and `build/ios` scaffolding that only builds under their own toolchains —
   confirmed locally that a bare `go build ./...` inside `cmd/pushapp-ui`
   fails on those for exactly that reason. A CI job must build the specific
   package (`.`) or use `wails3 build` with an explicit target, never `./...`,
   inside that directory.

## Verification

Once added: a PR that intentionally breaks `cmd/pushapp-ui` (e.g. a typo in
`pushservice.go`) should fail the new job on all platforms it's expected to
build on, and a clean PR should pass alongside the existing four-binary matrix
unaffected.

## Not in scope here

Actually packaging/signing/distributing the UI app (macOS notarization,
Windows code signing, Linux AppImage/deb) — this plan is only about CI
*building and vetting* it, matching the rigor the rest of the repo already
gets, not about shipping it.
