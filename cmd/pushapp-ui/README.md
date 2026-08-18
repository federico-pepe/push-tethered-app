# pushapp-ui

Desktop UI for the module host: lists the available modules, shows which is
active, switches between them, and installs/uninstalls process-loaded ones.
It owns the hardware exactly the way `cmd/pushapp` does — both go through
`internal/bootstrap` — and adds nothing to the module contract.

Wails v3. **This is a separate Go module** from the repo root (its own
`go.mod`, with two `replace` directives of its own); see the root
[CLAUDE.md](../../CLAUDE.md) for why, and for what that means when building.

## Commands

```bash
wails3 dev              # hot-reload window
wails3 build            # produces bin/pushapp-ui
```

Needs the `wails3` CLI and Node/npm on top of everything `cmd/pushapp` needs:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 doctor           # checks the rest of the per-OS toolchain
```

`wails3 build` produces a **bare executable**, on every OS — no `.app` bundle,
no installer. Those come from `wails3 package` instead (macOS `.app`/`.dmg`,
Linux AppImage/deb/rpm, Windows NSIS), which needs extra per-OS packaging
tooling and is not currently wired into CI. The icon'd, "Push Tethered App"-named
bundle that appears in `bin/` is `pushapp-ui.dev.app`, which `wails3 dev`
creates so macOS treats the dev build as a real app — not a build output.

## Configuration

**Project config is [`build/config.yml`](build/config.yml), not `wails.json`.**
Wails v3 replaced the v2 `wails.json`; any doc telling you to edit `wails.json`
is describing v2.

`build/config.yml`'s `info:` block feeds the generated build assets under
`build/darwin`, `build/windows` and `build/linux` (plists, NSIS defines,
version resources, nfpm metadata). After editing it, regenerate them:

```bash
wails3 task common:update:build-assets
```

That **overwrites** hand-edits to those generated files, so make changes in
`config.yml` and regenerate rather than editing the assets directly.

## Desktop only

The Wails template ships `ios/` and `android/` targets. Both were deleted
(2026-08-18) and their `includes:` removed from `Taskfile.yml`: this app owns
a USB-attached Push over libusb, which no mobile OS allows.

`wails3 task common:update:build-assets` still regenerates `build/ios` assets
unconditionally, so `build/ios` and `build/android` are gitignored. It does
*not* regenerate the build-tagged `main_ios.go` / `main_android.go` that used
to make `go build ./...` fail in this directory, so that now works — though
`wails3 build` remains the command that produces a usable binary.
