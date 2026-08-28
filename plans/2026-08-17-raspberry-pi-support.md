# Raspberry Pi support

**Status: closed 2026-08-18, confirmed on Pi 5 hardware.** Full detail in
[docs/platform/linux.md](../docs/platform/linux.md)'s Raspberry Pi section.
Kept here as reasoning trail — see that doc for the current facts.

## Context

Federico asked whether the Linux build would run on a Raspberry Pi. Target is
**Pi 4/5** — the good case: proper USB 3.0 controller rather than the shared
internal hub older Pis (Zero, 1, some Pi 3 paths) route USB through, and a
quad-core Cortex-A72/A76 that should have real headroom for a 960×160 UI at
30fps. This plan exists so that assessment doesn't have to be redone from
scratch later.

## Why this isn't just "already covered by the Linux build"

The existing Linux confirmation (`docs/archive/feasibility.md` §Linux, Mint x86_64) does
not extend to Pi automatically, for the same reason nothing cross-compiles in
this project: `gousb` and `rtmididrv` are both cgo, and the CI matrix's
`ubuntu-latest` artifact is `x86_64` — a different architecture, won't run on a
Pi at all. **Building for Pi means building natively on a Pi**, same rule as
every other target in this project (see CLAUDE.md's cross-platform builds
section).

## Expected to just work

Same steps as the confirmed Mint build:

```bash
sudo apt install libusb-1.0-0-dev libasound2-dev pkg-config build-essential
# clone ableton-push-hack + push-tethered-app at the matching relative path,
# or edit go.mod's replace directive to match wherever they land
cd push-tethered-app
go build -o pushapp ./cmd/pushapp
```

Same udev rule as documented in CLAUDE.md for non-root USB access, including
the two gotchas already hit once on Mint:
- a `sudo tee` inside a heredoc/piped command can silently fail to write the
  rule file — verify with `cat /etc/udev/rules.d/50-push.rules` after creating it
- `udevadm trigger` does not retroactively fix a device that already
  enumerated under the old permissions — a physical unplug/replug is required

## Known unknowns — genuinely unverified, not assumed

1. **Go toolchain version.** `go.mod` requires `go 1.26.3`. Raspberry Pi OS's
   `apt` Go package tends to lag upstream significantly and may be too old,
   which would fail before cgo is even reached. Check `go version` first; if
   too old, install from the official arm64 tarball at go.dev/dl rather than
   `apt`.
2. **32-bit vs 64-bit OS.** Use **Raspberry Pi OS 64-bit (`arm64`)**, not the
   32-bit `armhf` image. Nothing here has been tested under 32-bit cgo, and
   arm64 is the better-supported, more likely to just work target.
3. **Sustained frame rate under Pi's weaker CPU.** `core/display.ToBGR565`
   encodes via per-pixel `image.At()` — 153,600 interface calls per frame.
   Flagged back in `docs/archive/feasibility.md` §9.3 as the first likely bottleneck
   even on a laptop; unknown whether Pi 4/5 holds 30fps or needs a lower `-fps`
   value. Measure, don't assume.
4. **USB behaviour specific to the Pi 4/5 controller.** Should be fine — proper
   USB 3.0 controller, not the shared hub — but not actually measured. Watch
   for the same things checked on other platforms: does `cmd/probe` enumerate
   cleanly, does `cmd/frametest` hold 30fps without drops, does the
   `LIBUSB_ERROR_ACCESS`/co-existence behaviour with Live (irrelevant here,
   Pi's not running Live, but worth confirming Push itself behaves identically)
   match what macOS/Mint showed.
5. **USB power supply margin.** Push is bus-powered. Pi 4/5 USB 3.0 ports
   supply enough per spec, but a marginal power supply combined with other
   attached peripherals is a real-world variable worth ruling out if anything
   behaves flaky, before chasing a software explanation.

## CI

Not added yet. GitHub does offer `arm64` Ubuntu-hosted runners, but
availability on this repo's specific plan has not been confirmed — check before
assuming a `linux/arm64` matrix entry is free to add. Do this only after manual
Pi testing succeeds; no point automating a target that hasn't been proven once
by hand.

## Next step, whenever this gets picked back up

1. Confirm `go version` on the Pi meets `go.mod`'s requirement; install from
   go.dev if not.
2. Run through the standard build steps above.
3. `./probe` first (read-only, safest first test).
4. `./pushapp -fps 30`, watch actual frame rate and screen output, same
   verification discipline as every other platform in this project — exit 0
   does not mean it rendered correctly (see the hardware-test-loop discipline
   already established for this project).
5. If 30fps struggles, try lower `-fps` before concluding anything about the
   architecture generally.
6. Record results in `docs/archive/feasibility.md`, following the same "measured, not
   inferred" standard as the macOS/Linux sections.

## Results, 2026-08-18

Tested on a **Pi 5** (Debian 13 "trixie", 64-bit arm64), not Pi 4. Answers to
the five "known unknowns" above:

1. **Go toolchain.** Sidestepped rather than answered — no Go installed on
   the Pi at all. Built natively on GitHub's `ubuntu-24.04-arm` hosted runner
   instead (a new `build-pi` CI job) and copied the resulting binaries over.
   Real aarch64 hardware, not emulated, so this satisfies the same
   no-cross-compile rule the per-OS CI matrix already relies on — and means
   Pi OS's `apt` Go version never matters.
2. **64-bit confirmed.** Already running Pi OS 64-bit; not tested on 32-bit,
   still not recommended.
3. **Frame rate: not a problem.** `frametest` held 29.9fps, `pushapp`'s
   monitor module held 29.8fps — both effectively identical to the confirmed
   macOS/Windows numbers. `ToBGR565`'s per-pixel cost was the suspected
   bottleneck and did not materialize as one on Pi 5's CPU.
4. **USB behaviour: clean.** `probe` enumerated Push 3 with all 7 interfaces
   correctly; `frametest` and `pushapp` both claimed interface 0 without
   incident once the udev rule existed. One thing *not* pre-existing on a
   fresh Pi OS install: the udev rule from
   [docs/platform/linux.md](../docs/platform/linux.md) had to be created by
   hand — first `frametest` run failed `LIBUSB_ERROR_ACCESS`, exactly the
   failure mode that doc predicts, and needed a physical replug afterward
   (the doc's other documented gotcha) before it took effect.
5. **Power margin.** Not an issue in this test — no flakiness observed. Still
   unmeasured under heavier peripheral load.

Not tested: Pi 4 specifically, multi-hour endurance, headless kiosk `-module`
flag under real use. `cmd/pushapp-ui` is now built for arm64 too (folded into
`build.yml`'s main matrix 2026-08-27, alongside the CLI tools that were
already there — no longer a separate `build-pi` job, and no longer skipped
for the Pi) — but that's CI compilation only. Running the Wails/webkit2gtk UI
on real Pi hardware is still untested.
