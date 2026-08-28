**Status (updated 2026-08-27): the audio question is closed and solved.
Schwung integration itself is open — see "Next steps" at the bottom of
this file.** Everything above that section is the raw exploration
trail: several dead ends, then a working answer for audio. Read it for
the "why" behind each decision. The audio implementation itself lives
in `ableton-push-hack`, not here: see
`ableton-push-hack/plans/2026-08-27-push-audio-virtual-device.md` and
the hack at `ableton-push-hack/hacks/push-audio-loopback/`.

# Feasibility: running Schwung on Push 3 standalone

## Context

Federico asked whether Schwung (a third-party framework that runs custom
synths/effects/controllers on Ableton Move, github.com/charlesvestal/schwung)
could be cross-compiled and run on Push 3 in **standalone** mode instead,
with display, MIDI, and audio all attached. This is a research question
(no code change requested yet) — the plan below is the answer plus a
recommended path if he wants to prototype it.

This repo (`push-tethered-app`) and its sibling `ableton-push-hack` only
talk to Push 3 in **tethered/controller** mode over USB from a host
computer. Standalone-mode hacking on Push 3's own onboard Linux is a
different surface, documented in `ableton-push-hack` (SSH-based
deployment to the device itself), and that's the surface Schwung would
need.

## What the two devices actually are

**Ableton Move** (Schwung's target): ARM64 (Raspberry Pi CM4), presumably
its own board-specific display/audio/input wiring.

**Push 3 standalone** (confirmed via `ableton-push-hack/docs/push3-internals.md`):
- **x86_64 Intel**, `AbletonOS abletonos-x86_64-intel-v3.21`, RT-patched
  Linux kernel, A/B NVMe rootfs (SWUpdate), sysvinit.
- All physical I/O (display, audio, MIDI, LEDs, pads) is behind an
  onboard **XMOS co-processor**, reached over an *internal* USB bus — not
  a direct SoC peripheral the way Move's screen/DAC likely are.
- Display: Push3's own app renders directly to DRM/KMS (Qt5/EGLFS,
  `--faceless`); a separate Xorg :0 runs for other X11 clients. A
  third-party process gets the physical screen either by hooking the
  XMOS bulk-transfer calls (`ableton-push-hack/hacks/push-display/src/push_hook.c`,
  `LD_PRELOAD` on `libusb_bulk_transfer`) or by being an X11 client.
- MIDI/pads: exposed as a normal **ALSA sequencer** client (`/dev/snd/seq`,
  ports `Live Port`/`User Port`/`External Port`) — no libusb needed.
- Audio: XMOS interfaces present as class-compliant USB-Audio, but
  **neither repo documents a standalone-side ALSA device name, or any
  proof a third-party process can open/control it.** This is the one
  unverified piece.
- Deployment: SSH, no package manager — static binaries pushed to
  `/data/push-hack/` (writable, survives OS updates), registered as
  sysvinit scripts. `/opt` (rootfs) is read-only. An installed hack must
  be removed before any OS update or the update can hang the device.
- Cross-compiling Go binaries (`GOOS=linux GOARCH=amd64`) already works
  and is exactly how `ableton-push-hack` deploys today.

## `schwung-spi` and the real architectural difference (2026-08-27)

The Schwung author published a second, separate repo,
`github.com/charlesvestal/schwung-spi` — a two-file C library (per its
README) that hooks `open()`/`mmap()`/`ioctl()` to intercept Move's SPI
communication with its XMOS audio/IO processor, "derived from Ableton's
`ablspi` library (GPL-2.0, obtained via GPL source request for Move's
JACK driver)" — i.e. exactly the same kind of GPL-source archaeology used
on Push 3 above, applied to Move instead. It exposes a "shadow copy" of
the SPI buffer (`schwung_spi_get_shadow()` vs `schwung_spi_get_hw()`) so
third-party code can read/write **audio, MIDI, and display together**,
because on Move they're all multiplexed into **one shared buffer**: a
fixed byte layout per ~2.9ms/128-frame cycle at 44.1kHz (audio OUT at
offset 256, audio IN at offset 2304, both stereo int16).

This is the key fact that explains, rather than contradicts, everything
found on Push 3: **Move's audio is not a USB-Audio-class ALSA device at
all** — it's raw PCM at a fixed offset in a proprietary buffer, with no
kernel audio-class driver anywhere in the path. That's why hooking it is
trivial and lossless on Move: there is no exclusivity to fight, because
there is no real audio driver arbitrating it. Push 3's XMOS audio path is
architecturally different — it's a genuine class-compliant `snd-usb-audio`
device (confirmed via `aplay -l`, confirmed no Ableton-specific quirk in
the GPL kernel source, see below) with real kernel arbitration, which is
exactly why it exclusively locks and why no raw-buffer shortcut sits
underneath it the way Move has.

Display is the one piece that *is* architecturally the same pattern on
both devices: Push 3's screen also rides a raw vendor USB channel outside
any OS class driver, which is exactly why `push_hook.c`'s `LD_PRELOAD` on
`libusb_bulk_transfer` already works — same technique as `schwung-spi`,
just intercepting USB bulk transfers instead of SPI ioctls, independently
discovered on each device.

**Net effect on this plan: no change to the recommended path, but a
clear reason why it's shaped the way it is.** Display porting = reuse the
existing `push_hook.c` pattern (same technique family as `schwung-spi`).
Audio porting = there's no Move-style raw-buffer shortcut to find on
Push 3, however long the search — Link Audio (already confirmed working
live) is Push 3's actual sanctioned equivalent of what `schwung-spi` gets
for free via raw hardware access on Move.

**Schwung's stack — corrected after reading the actual source
(2026-08-27, downloaded to `~/Developer/schwung-main`).** The earlier
picture from its sparse README/docs was wrong on the essentials:

- Go is not the DSP host — it's `schwung-manager/`, a separate web/
  repair server, unrelated to audio.
- JavaScript (~117 files, `src/shadow/`, `src/shared/`) is UI-only,
  running inside a vendored QuickJS interpreter for the "Shadow UI" —
  not audio.
- The actual **DSP engine is plain, portable C**: `src/modules/audio_fx/*`
  (e.g. `freeverb.c`, a from-scratch Schroeder-Moorer reverb, no DSP
  framework) and `src/modules/midi_fx/*`, loaded as `.so` plugins via a
  custom stable ABI (`src/host/plugin_api_v1.h`/`audio_fx_api_v2.h`). No
  NEON intrinsics, no ARM asm anywhere in this code — genuinely portable
  to x86_64 as-is. JACK2 is vendored wholesale (`src/lib/jack2/`) as the
  audio graph/session layer.
- **There is no hardware abstraction layer.** No `src/hw/`,
  `src/platform/`, nothing to target with a "Push 3 adapter." Every
  Move-specific access is hardcoded into one 8,692-line file,
  `src/schwung_shim.c` — an `LD_PRELOAD` shared object injected into
  **Move's own proprietary firmware binary**, `MoveOriginal`. It hooks
  libc `ioctl`/`mmap`/`sendto`/`send`/`sd_bus_*` via
  `dlsym(RTLD_NEXT, ...)` to intercept Move's own SPI/audio/MIDI/D-Bus
  traffic in place (`docs/ARCHITECTURE.md`).
- Display is not framebuffer/DRM on Move either — it's one field inside
  a proprietary 768-byte SPI transaction to a **Move-specific custom
  kernel driver**, `/dev/ablspi0.0` (`ABLSPI_WAIT_AND_SEND_MESSAGE_WITH_SIZE`
  ioctl, `docs/SPI_PROTOCOL.md`), backed by Ableton's own out-of-tree
  `ablspi-{core,gpio,proc}.c` driver + device-tree overlay. Display,
  MIDI, and audio are **not separated at all** on Move — same transfer,
  same buffer, same ioctl.
- Build: no CMake, a `Dockerfile` cross-compiles everything with
  `aarch64-linux-gnu-gcc/g++` (title: "Targets: Ableton Move (aarch64
  Linux)"). No CI workflow present in this download.
- **`libs/link` is a genuine git submodule of `github.com/Ableton/link`**
  (confirmed via `.gitmodules`), not a reverse-engineered library — this
  matches and explains the Link Audio breakthrough below.

**The critical correction: Push 3 has no equivalent of `MoveOriginal` or
`ablspi` to hook.** Schwung's actual display+MIDI mechanism is a parasite
on one specific proprietary Move binary and one specific Move-only kernel
driver — not a recompile target, not an adapter target, nothing to port
at all for that part. Only two pieces of Schwung are genuinely reusable
on Push 3: the DSP `.so` plugins (stable C ABI, architecture-portable),
and the Link Audio subscriber pattern (`src/host/link_subscriber.cpp`),
which — per the breakthrough section below — already talks to Push3's
`Live` today, because it was already Move-independent by Schwung's own
design (it goes through the public Link Audio protocol, not through
`schwung_shim.c`/`ablspi` at all).

## Feasibility verdict (revised after reading the real source)

This is not a "cross-compile plus adapter" job. `schwung_shim.c` cannot
be ported at all — it hooks a Move-only proprietary binary and a
Move-only kernel driver that don't exist on Push 3. The real feasibility
question is: **can a brand-new Push3 host, reusing only Schwung's
portable DSP `.so` plugins and its Link Audio pattern, be built on top
of infrastructure this repo and `ableton-push-hack` already have?**

1. **MIDI/pads — easy, already-solved territory.** Both devices expose
   pads/encoders as an ALSA sequencer client. This repo's own
   `internal/midiin`/`internal/midiout` already do exactly this against
   Push 3. No Schwung code is reusable here (there was never an
   abstraction to reuse) — a new host writes this directly against
   Push3's ALSA seq ports, which is a solved problem in this codebase
   already.
2. **Display — medium difficulty, known path, but entirely new code.**
   Schwung's display code cannot be reused (it's the `ablspi` SPI
   protocol, Move-only). Push 3's screen is reachable via the
   `push_hook.c` LD_PRELOAD trick already proven in `ableton-push-hack`
   (hooking `libusb_bulk_transfer` to the XMOS chip), or by becoming an
   X11 client. A new host would draw Schwung's "Shadow UI" output
   through this path from scratch — real from-zero work, not porting.
3. **Audio — confirmed blocker for the ALSA path, but Link Audio
   sidesteps it entirely.** (verified live on hardware, 2026-08-27):
   `aplay -l`/`arecord -l` show the XMOS chip as a normal ALSA card
   (`card 0: A3 [Ableton Push 3], device 0: USB Audio`), and it is *not*
   held by any visible process fd — a scan of every `/proc/*/fd/*` found
   only `/dev/snd/seq` and `/dev/snd/controlC0` open, both by Push3's own
   process (PID 954). But `speaker-test -D hw:0,0 ...` (mono and stereo,
   `-l 1 -p 1`) both fail immediately with `Playback open error: -16,
   Device or resource busy`, while Push3/Live run normally (standalone,
   not tethered). The lock is not a plain ALSA PID-held-fd lock a second
   process could wait out or share via dmix — it sits below that, most
   likely the XMOS USB-Audio driver reserving the endpoint for Push3's
   own audio session even at idle. **A third-party standalone process
   cannot get direct PCM access to Push 3's audio output while
   Push3/Live's own stack owns the device**, at least not without killing
   or reconfiguring that stack first (unexplored, and risky — see
   `docs/protocol/usb-and-safety.md`'s caution against undocumented XMOS
   control-interface writes).

## GPL kernel source findings (2026-08-27)

Ableton published GPL source for Push 3 firmware v2.4.2; a local copy
sits at `ableton-push-hack/resources/push-assets/push3-242-gpl-sources.tgz`
(725MB, gitignored, Yocto/OpenEmbedded dump). Mined the kernel package
(`linux-push3-5.15.48+gitAUTOINC+4ec79de9ce-r0`) for the audio-locking
question:

- `sound/usb/quirks-table.h` / `quirks.c` have **no Ableton-specific
  entry** — the USB-Audio driver is plain upstream code. The `busy` lock
  we hit live is standard ALSA single-open hardware-device semantics,
  not a custom vendor patch.
- `kernel.config` has **`CONFIG_SND_ALOOP=m`** — ALSA Loopback (a virtual
  sound card pair) is enabled in this exact kernel's config.
- But `/lib/modules/5.15.48-intel-pk-preempt-rt` on the actual device has
  **no `snd-aloop.ko`** — confirmed via `modprobe snd-aloop` failing with
  "Module not found." Ableton didn't ship it, despite the kernel
  supporting it.
- Module signing is on but not enforced (`CONFIG_MODULE_SIG=y`,
  `CONFIG_MODULE_SIG_FORCE` unset) — an out-of-tree, unsigned module
  built against this exact kernel source/config would still load via
  `insmod` (taints the kernel, isn't rejected).
- Live's `Preferences.cfg` (binary format, `/data/.config/Ableton/Live
  <version>/`) contains an `AlsaAudioDevice` / `List<AudioDevice>` /
  `PrefDeviceList` model. Its MIDI side (`MidiIO`) is proven to enumerate
  arbitrary ALSA client names dynamically (`DDJ-FLX4 MIDI 1`, `Checker
  port` both appear) — strong circumstantial evidence Live's audio
  device list works the same generic way and would pick up a new ALSA
  card automatically.

## BREAKTHROUGH: Link Audio, confirmed live (2026-08-27)

Built the official `Ableton/link` SDK's `LinkAudioHut` example natively on
a Mac on the same LAN (repo: `github.com/Ableton/link`, public,
GPLv2/proprietary-dual-licensed, `include/ableton/LinkAudio.hpp` is real
and documented as "cross-device shared tempo, quantized beat grid, and
sharing audio" — not something Schwung reverse-engineered). With "Link
Audio" enabled on Push3's `Live` (via its own UI), ran the built
`LinkAudioHut` binary with no code changes from upstream and got, over
plain network discovery, no code on Push3 at all:

- **`numPeers: 1`**, tempo `120.00` matching Push3's Live, beat/phase
  clock advancing in lockstep — standard public Link tempo sync working
  end-to-end against Push3's `Live`.
- **`LinkAudio Peers:` `Move | 1-MIDI`, `Move | 2-MIDI`, `Move | 3-MIDI`,
  `Move | 4-MIDI`, `Move | Main`** — Push3's `Live`, in standalone mode,
  announces its Link Audio peer name as **literally "Move"**, with the
  exact same 4-track + Main channel layout Move itself uses.

This means Schwung's actual `link_subscriber.cpp` filter
(`pc.peerName == "Move"`, channel names `"1-MIDI"` etc., see the earlier
GPL/architecture research above) would very plausibly match Push3
**unmodified**. The whole earlier ALSA/XMOS exclusivity investigation
(kill-and-race, `snd-aloop` kernel module, `Preferences.cfg` editing) was
solving the wrong problem — Live never intended third-party audio access
to go through ALSA at all on this hardware line. Link Audio is the
sanctioned path, publicly documented, and it works today, from a
completely unmodified stock build, with nothing installed on Push3.

Confirmed via: built `LinkAudioHut` from `github.com/Ableton/link`
(`examples/linkaudiohut/main.cpp`) on macOS
(`cmake -DLINK_BUILD_EXAMPLES=ON .. && cmake --build . --target
LinkAudioHut`), ran it with Link + Link Audio enabled via its own `a`/`c`
key bindings, observed peer discovery and the channel list printed by
`state.link.channels()` via `setChannelsChangedCallback`.

**Untested next questions:**
- Does Push3's Link Audio channel actually carry live audio payload
  (via `LinkAudioSource`/`BufferHandle`), or only advertise channel
  *presence* while the audio-transport path is silently a no-op on this
  hardware? (`LinkAudioHut`'s "source (buffered)" column would show
  this — worth re-running with a track actually playing audio into a
  channel and watching that field, plus wiring up an actual
  `LinkAudioSource` subscriber instead of just printing channel names.)
- Does this require being on the same LAN/subnet as Push3 (multicast
  discovery, UDP 20808, group `224.76.78.75` — confirmed reachable from
  this Mac), or could a process running on Push3 itself join the
  loopback interface just as easily (likely yes, simpler, no network
  dependency for what would ultimately want to run co-located anyway)?
- Whether Schwung's exact code needs zero changes, or the "Move" peer
  name is coincidental fixed firmware behavior that would need
  confirming isn't tied to some other Move-only artifact once real audio
  payload is attempted.

## RESULT: `snd-aloop.ko` built and loaded successfully (2026-08-27)

Built and tested this end to end, per Federico's preference to try this
before Link Audio. Full success:

- Extracted the full patched kernel source (124MB) from the GPL tarball,
  copied `kernel.config` in as `.config`, ran `make olddefconfig` and
  `make modules_prepare` in a `debian:bullseye` container (needed
  `build-essential bc bison flex libssl-dev libelf-dev kmod rsync`),
  then `make M=sound/drivers modules` to build just `snd-aloop.ko`
  (`sound/drivers/aloop.c` in-tree).
- Confirmed vermagic matches **exactly**: built module says
  `vermagic=5.15.48-intel-pk-preempt-rt SMP mod_unload`, identical to
  the string pulled from a real shipped `.ko`
  (`/lib/modules/.../snd-usb-audio.ko`) on the device — no `--force`
  needed. (Version string derived from the kernel `Makefile`'s
  `VERSION.PATCHLEVEL.SUBLEVEL = 5.15.48` plus
  `CONFIG_LOCALVERSION="-intel-pk-preempt-rt"`,
  `CONFIG_LOCALVERSION_AUTO` unset — no git-hash suffix to chase.)
  `modpost` printed the expected "unresolved symbol" warnings (only
  built one subdirectory, not the whole kernel, so it has no full
  `Module.symvers`) — harmless, since `CONFIG_MODVERSIONS` is off on
  this kernel and symbol resolution happens against the real running
  kernel's export table at `insmod` time, not against build-time CRCs.
- `scp`'d to `/tmp` and `insmod`'d as root: **exit 0, clean load.**
  `/proc/asound/cards` immediately showed a second card,
  `1 [Loopback]`, alongside the untouched `0 [A3]`. `dmesg` showed only
  the expected taint warnings (out-of-tree, unsigned — exactly the
  behavior predicted from `CONFIG_MODULE_SIG_FORCE` being unset).
- **Self-test confirms real audio flows through it**: played a 440Hz
  sine into `hw:Loopback,0,0` while recording from the paired
  `hw:Loopback,1,0` (`aplay -L` shows it as a normal `Loopback` device
  with the usual `front`/`surround*`/`iec958` variants). Recorded WAV:
  `peak=26215` (16-bit signed, real signal, not silence), `66122/176400`
  nonzero samples — exactly consistent with a ~1.5s sine burst inside
  the 2s capture window. One caveat hit and resolved: the two ends of a
  Loopback pair must open at the same sample rate or the second side
  gets `Invalid argument` — matched both sides at 44100 Hz.
- Zero impact on the real hardware path: `A3` card, `Live`, and the
  existing Push3 process were untouched throughout.

**What's proven:** a virtual ALSA card genuinely works on Push 3, is
open now, and carries real audio.

## RESULT: full round trip confirmed audible on real hardware (2026-08-27)

Closed out every open question from the section above, live:

- **Live's on-device UI does list "Loopback" as selectable** — no
  `Preferences.cfg` editing needed at all. Federico saw it directly on
  Push3's own screen, in both the input and output device pickers (it
  appears twice because `snd-aloop` creates two cross-wired devices,
  device 0 and device 1 — expected, not a bug).
- He selected it as an **input**. Confirmed on-device:
  `/proc/asound/card1/pcm0c/sub0/status` showed `state: RUNNING,
  owner_pid: 2162` (Live) — Live was actively capturing from
  `hw:Loopback,0,0` at `32 channels, S16_LE, 44100Hz`
  (`/proc/asound/card1/pcm0c/sub0/hw_params`). Live's actual **output**
  stayed on the real hardware the whole time
  (`card0/pcm0p` also `RUNNING`, same owner_pid, `hw:0,0` still
  exclusively busy to any outside opener) — nothing about the real
  output path changed.
- Played a 440Hz test tone into the cross-wired partner,
  `hw:Loopback,1,0`, matching Live's exact negotiated format
  (`speaker-test -D hw:Loopback,1,0 -c 32 -r 44100 -f S16_LE`), per the
  device-0↔device-1 pairing already established in the first self-test.
- **Federico heard it come out of Push 3's real speaker/headphone
  output** — "glitchy sound," but audible. This is the actual proof:
  audio generated by a completely separate process reached the physical
  hardware output, through Live, on a stock, unmodified Push 3 build,
  using nothing but a self-built ALSA Loopback module. Nothing needed
  disabling on the Push3 side — the missing piece was purely a Live Set
  configuration one (an audio track, Input = Loopback, Monitor = In,
  routed to Master), which is how any external-input monitoring works
  in Live normally.
- **The glitching has an ordinary, fixable explanation**: `speaker-test`
  and Live's audio engine are independent processes sharing one ALSA
  Loopback ring buffer with no shared clock. Live's side runs tight
  real-time 128-frame periods; `speaker-test` used generic default
  timing. A purpose-built streaming client — matching
  `period_size=128`, `SCHED_FIFO` priority, continuous non-blocking
  writes timed to `avail_min` — would clean this up. This is a known,
  solvable ALSA loopback timing-discipline problem, not evidence the
  approach is unsound.

**Net effect: this closes the audio question for this plan.** Every
piece needed for a real Schwung-on-Push3 audio path — display (proven
route via `push_hook.c`), MIDI (already solved in this codebase), and
now audio (proven live, audibly, on real hardware) — has a working,
demonstrated mechanism. What remains is engineering: a proper low-jitter
streaming client instead of a one-shot `speaker-test` call, and the
actual DSP-plugin-hosting work described below.

## RESULT: glitch root-caused and fixed (2026-08-27)

Built `hacks/push-audio-loopback/src/loopback_feed.c` in
`ableton-push-hack` (branch `feature/push-audio-loopback`) — a small
tool matching Live's negotiated `hw_params`, writing via blocking
`snd_pcm_writei` (ALSA's own backpressure paces it) with `SCHED_FIFO`
priority. First run: **0 xruns reported on either the writer or the real
hardware PCM, `dmesg` clean — yet still audibly glitchy.** That ruled out
a simple buffer-starvation bug and pointed at something below the ALSA
xrun-detection layer entirely.

Root cause, found in `snd-aloop`'s own source: this kernel is
**`CONFIG_HZ=250`** (4ms jiffies tick), and `snd-aloop`'s default clock
source is explicitly the jiffies timer (its own module-param doc:
`"Empty string for jiffies timer [default]"`). Live's negotiated period
was 128 frames (~2.9ms) — shorter than the 4ms tick driving the
Loopback's virtual clock, so audio arrived in lumpy ~4ms bursts instead
of a smooth stream. This never trips an xrun (data is always
"available" whenever polled), it just isn't delivered on the cadence a
tight real-time consumer expects — exactly the kind of artifact that's
invisible to buffer-level diagnostics but audible.

Tried the driver's own escape hatch first: `snd-aloop` supports a
runtime-writable `/proc/asound/PHVAudio/timer_source` to bind its clock
to a real hardware PCM's timer instead of jiffies. **Dead end,
confirmed via source read**: `loopback_snd_timer_open()` hardcodes
`dev_class = SNDRV_TIMER_CLASS_PCM` — it can only reference another PCM
device's timer, never a global one (ALSA does expose a genuine
high-resolution global timer, `G3: HR timer` in `/proc/asound/timers`,
but this driver's parser has no path to reach it). Pointing it at
Push3's own hardware (card 0) failed immediately with `Input/output
error` after one buffer — expected in hindsight, since most USB-audio
drivers (including this one) don't back a real ALSA timer object at
all; they do their own internal URB-completion timing. Reverted
`timer_source` back to empty (jiffies) — this whole avenue is closed
without a full kernel rebuild at a higher `CONFIG_HZ`, which is a much
bigger, riskier step than anything else in this plan and not
recommended without a much stronger reason.

**The actual fix, the standard/expected one for this exact class of
problem: increase the consumer's buffer size.** Federico raised Live's
Buffer Size in its own Audio Preferences; the Loopback capture
re-negotiated to `period_size=512, buffer_size=1536` (was
`128`/`384`). A 512-frame period safely spans multiple 4ms jiffies
ticks instead of falling inside one, which absorbs the coarse timer
granularity. Reran `loopback_feed` (0 xruns, unchanged) — **Federico
confirmed a clean tone, no more glitching.**

This is now a fully validated, working audio path end to end: real
audio, generated by an independent process, reaches Live and then
Push3's physical output, audibly clean, on a stock Push3 unit plus one
self-built (and self-renamed) kernel module. Latency cost of the fix:
a larger buffer (512 vs 128 frames, ~11.6ms vs ~2.9ms) — a normal,
expected trade for stability on this timer, not a regression to chase
further unless a specific latency budget demands it later.

**This is where the settled, documented outcome now lives**: renamed
to "Push Hack Virtual Audio," built and committed at
`ableton-push-hack/hacks/push-audio-loopback/`
(branch `feature/push-audio-loopback`, pushed), with its own distilled
plan at `ableton-push-hack/plans/2026-08-27-push-audio-virtual-device.md`
and a durable protocol-facts writeup in
`ableton-push-hack/docs/push3-internals.md`.

## Can we create a virtual sound card and route Live's audio through it,
## the way we did for MIDI? (original write-up, kept for the reasoning
## trail — see RESULT above for what actually happened)

**In principle, yes — this is now the most promising path**, more so
than fighting the XMOS hardware lock directly. What's proven vs. still
open:

- Proven: the kernel supports ALSA Loopback; the module just isn't
  present on-device; module-loading isn't blocked by signature
  enforcement; Live's preferences format generically models ALSA audio
  devices the same way it models the MIDI ports we already exploit.
- Open / unbuilt:
  1. **Build `snd-aloop.ko`** against this exact kernel source + config
     (both now extracted locally) — same cross-build discipline
     `ableton-push-hack` already uses for other binaries, but this is a
     kernel module, not a userspace binary, so it needs a matching
     toolchain and exact kernel version match.
  2. **`insmod` it as root** over SSH and confirm a new "Loopback" card
     shows up in `aplay -l` without disturbing the existing `A3` card.
  3. **Get Live to actually select it as output.** Unknown whether Push3
     standalone exposes any on-device UI/menu for Live's audio
     preferences at all (it may be hidden, since the device ships fixed
     to its own hardware) — if not, the only route is writing directly
     into the binary `Preferences.cfg`, which has no published schema
     and is a materially riskier edit (a bad write could corrupt Live's
     prefs on that unit).
  4. **A consumer for the loopback's capture side.** Loopback is just a
     virtual patch cable between two ALSA clients — it doesn't produce
     sound on its own. Something needs to read the capture side and
     either re-route it to the real XMOS output (defeats the purpose,
     since that's still locked by Live) or ship it elsewhere (network
     stream, or wait for Live to fully release `hw:0,0` per the earlier
     kill-and-race finding, then have Loopback's other end feed the real
     hardware once free).

This still doesn't cleanly solve "Schwung's synth plays out of Push 3's
own speaker while Live also runs" — it solves "get audio out of a
process running alongside Live on the same box," which is the real goal.
Whether that's the same problem depends on what Federico actually wants
Schwung's audio to reach (the physical Push 3 output vs. anywhere else).

## BREAKTHROUGH 2: the LD_PRELOAD path is already running inside `Live`
## right now (2026-08-27)

Re-read `schwung-spi`'s actual source (not just its README) and
`ableton-push-hack/hacks/push-display/src/push_hook.c` in full. The real
mechanism behind both is the same, and it turns out to already be live on
Push 3, for a reason nobody had traced through before:

- `schwung-spi`'s technique is not "hack the SPI protocol" — it's "inject
  `LD_PRELOAD` into the *process that owns the device handle*, and
  intercept the library calls it already makes." It hooks `open()` (string
  match on `/dev/ablspi0.0`), `ioctl()` (match on the specific
  `SCHWUNG_IOCTL_WAIT_SEND_SIZE` request), and `mmap()` — returning a
  shadow buffer to the caller instead of the real one, syncing real
  hardware transfers around it.
- `push_hook.c` (already-shipping code in `ableton-push-hack`) does the
  identical pattern on Push 3 today: `LD_PRELOAD` set in
  `/etc/init.d/push3` before the launch line, hooking
  `libusb_bulk_transfer`/`libusb_submit_transfer` (display) — and it
  **already hooks a real ALSA library call**, `snd_seq_event_input`, to
  filter pad/button MIDI events. This is proof, not theory: hooking
  alsa-lib from inside this exact codebase, on this exact hardware,
  already works in production.
- The scoping check in `push_hook.c` is `hook_active = (strcmp(comm,
  "Push3") == 0)` — everywhere else the interposers are no-ops, per its
  own comment: "the init.d LD_PRELOAD line is inherited by every push3
  child (the Python launcher, PushWebServiceCli, …)."

**Traced the actual process tree live:** `/etc/init.d/push3` does
`export LD_PRELOAD=.../push_hook.so` immediately before
`start-stop-daemon` spawns `push3-launcher.py*` (PID 768,
`XPython3Exe`). `Live` (PID 1752) is a **direct child of that launcher**
(`ps -o pid,ppid`: 1752's parent is 768). Environment variables set
before `exec` inherit down the whole process tree unless a child clears
them — so **`push_hook.so` is already loaded inside `Live`'s own process
right now**, on stock, unmodified Push 3. The only reason it does nothing
there today is the `comm == "Push3"` scoping check, which is a one-line
change away from also covering `Live`.

**Confirmed `Live` is a real interposable target for audio, not just
MIDI:** `strings -a /opt/push3/Live` shows it dynamically links
`libasound.so.2` (present on-device at `/usr/lib/libasound.so.2.0.0` —
not statically linked in) and calls `snd_pcm_open`,
`snd_pcm_hw_params_*`, and critically **`snd_pcm_mmap_begin`** /
**`snd_pcm_mmap_commit`** — the exact pair of calls that bracket every
buffer of real audio Live writes to or reads from the hardware PCM
device. Being genuine dynamic symbols resolved through a real shared
library (not inlined/static), they're cleanly hookable via
`dlsym(RTLD_NEXT, ...)`, the identical mechanism already proven on
`snd_seq_event_input`.

**What this means:** full bidirectional shadow-copy access to Push 3's
actual audio stream — record what Live is about to play, or overwrite
it before it reaches hardware — is achievable by extending the
already-running `push_hook.so`: add a `comm == "Live"` branch and two new
hook functions (`snd_pcm_mmap_begin`, `snd_pcm_mmap_commit`) shaped like
the existing `snd_seq_event_input` hook, backed by a shared-memory buffer
the same way `push_display_shm_t` already works for display. No new
injection mechanism, no new deployment step, no `/etc/init.d/push3`
change needed — the `LD_PRELOAD` line is already in place and already
reaches `Live`.

**How this compares to Link Audio:** Link Audio (confirmed working
earlier) is zero-touch on-device (one Live UI toggle, no code installed)
but only sees channels a user explicitly routes to Link Audio, and its
payload-carrying capability on Push 3 is still unconfirmed. The PCM-hook
approach is unconditional (works on Live's actual master output/input
regardless of any per-track routing) and gives real read **and** write
access — but it's more invasive: it means shipping modified code into
`Live`'s own address space via a `.so` this project already maintains
and already modifies `/etc/init.d/push3` to load. That's the same risk
category `push_hook.c` already operates in, not a new one, but it's not
nothing — a bug in the new hook runs inside Ableton's own DAW process,
not a sandboxed side process. Any implementation needs the same defensive
posture `push_hook.c` already uses (its `BOOT_GRACE_SECS` passive window,
scoped activation, fail-safe pass-through on any `dlsym` failure).

**Not yet done:** no code has been written or loaded for this. Next real
step, if pursued, is writing the two hook functions and testing
read-only shadow-copy logging first (mirroring how `push_hook.c` itself
started as passive logging before adding overlay/takeover modes) before
attempting any write-side audio injection.

## RESULT: tried it, blocked by `AT_SECURE` — `Live` has `cap_sys_nice`
## (2026-08-27)

Built the passive-logging version of the plan above (new `experiment/
live-pcm-shadow-hook` branch in `ableton-push-hack`: `comm == "Live"`
branch, `snd_pcm_mmap_begin`/`snd_pcm_mmap_commit` hooks, hex-dump
logging only, zero mutation — see that branch for the diff), built it
with the existing Docker/`gcc:12-bullseye` Makefile, deployed with the
existing `deploy.sh`, restarted Push3. Result: the hook loaded correctly
into every other push3 child (`Push3`, `PushWebServiceCli`, `amidi`,
`dbus-send`, `sh` — each printed the existing load-banner line), but
**`Live` never printed the load banner at all** — not a missing PCM log
line, the constructor itself never ran there.

Root cause, confirmed: `getcap /opt/push3/Live` → **`cap_sys_nice=ep`**.
(`Push3` and `XPython3Exe` both have no capabilities at all — confirmed
empty.) A binary with file capabilities triggers the kernel's
`AT_SECURE` exec mode, and glibc's dynamic linker deliberately ignores
`LD_PRELOAD` (and `LD_LIBRARY_PATH`/`LD_AUDIT`) for any such process —
documented, intentional hardening (`man ld.so`, "secure-execution mode"),
specifically to prevent library-injection privilege bridges like this
one. `Live` almost certainly carries `cap_sys_nice` so its audio thread
can get `SCHED_FIFO`/RT priority without running as root — and that
same capability is what silently defeats the hook. This also explains
why `/proc/<Live-pid>/maps` and `/proc/<Live-pid>/environ` were both
permission-denied earlier in this investigation even from the same uid:
secure-exec processes become non-dumpable automatically. Not a timing
bug, not a log-contention issue — a deliberate security boundary,
working as designed.

**What would be needed to get past this (not attempted, not
recommended):**
- Stripping `cap_sys_nice` from `/opt/push3/Live` (`setcap -r`) would
  let `LD_PRELOAD` through again, but at the cost of `Live` losing its
  own RT scheduling privilege — a real functional regression to Live's
  actual audio performance (risk of xruns/glitches) just to enable a
  hack, and it wouldn't survive an OS update any better than the
  existing `/etc/init.d/push3` patch does. Confirmed empirically that
  root *does* bypass this specific check (a root-owned process already
  has every capability, so exec'ing a capability-carrying binary as
  root never counts as "gaining" a new one) — but Federico does not
  want to run the Push3/Live stack as root for this, and that's the
  right call: it's a much bigger blast-radius change than anything else
  tried here.
- A ptrace-based injector (attach to the already-running `Live` process
  and force a remote `dlopen()` after exec, bypassing `ld.so`'s
  pre-exec check entirely) would sidestep `AT_SECURE`, but is a much
  heavier engineering lift and a meaningfully bigger blast-radius
  category than the LD_PRELOAD-only approach this project has used so
  far — not something to attempt without a much stronger reason than
  this exploration has produced.

**Net effect: this path is closed off, cleanly and for a well-understood
reason.** The deployed `.so` is harmless as left (it works exactly as
before for `Push3`'s own process; the new hooks are simply dead code in
`Live`'s context, since `Live` never runs anything from it). **The
virtual-sound-card path (above) is what actually shipped.**

## What a real port actually looks like

"Port Schwung" resolves to: **build a new Push3 host, reusing only
Schwung's DSP `.so` plugins, on top of infrastructure that is now
entirely built and proven.** Every piece has a working, demonstrated
mechanism — none of them are Schwung's own code, since Schwung's actual
host layer (`schwung_shim.c`) cannot be ported at all (it hooks a
Move-only proprietary binary and kernel driver, see above):

- **Audio in/out — solved.** The virtual sound card
  ("Push Hack Virtual Audio", `ableton-push-hack/hacks/push-audio-loopback/`)
  gives a new host a real, bidirectional ALSA device to open, separate
  from the exclusively-locked real hardware. Confirmed carrying real,
  clean audio end to end on real hardware.
- **MIDI — solved, already in this codebase.** This repo's
  `internal/midiin`/`internal/midiout` already do ALSA-seq MIDI against
  Push 3.
- **Display — known path, not yet built.** No Schwung code is reusable
  (that part is Move's proprietary `ablspi` SPI protocol). Push 3's
  screen is reachable via `push_hook.c`'s `LD_PRELOAD` on
  `libusb_bulk_transfer` (`ableton-push-hack/hacks/push-display/`), or
  by becoming an X11 client — a new host would draw through this path
  from scratch.
- **DSP — the one untested piece.** Schwung's `.so` plugins
  (`plugin_api_v1` ABI, `freeverb.c` etc.) are portable C with no
  NEON/ARM-specific code, but none has actually been recompiled for
  `x86_64` or loaded via `dlopen` yet.

## Next steps

In order, each one gating the next. Steps 1 and part of 2 are now done,
using Braids (`~/Developer/schwung-braids-main`) instead of freeverb —
Federico's choice, and a better test besides: a full multi-voice
polyphonic synth with MIDI note handling, not just a passthrough
effect.

1. **DONE (2026-08-28): recompiled a Schwung DSP plugin for `x86_64`
   and loaded it.** Braids (`src/dsp/braids_plugin.cpp`, a C++
   Mutable Instruments macro-oscillator port) needed **zero source
   changes** — no NEON/ARM-specific code, confirmed by a clean native
   `linux/amd64` build in a plain `debian:bullseye` container (same
   approach already used for `push-audio-loopback`).

   Real finding along the way: this module implements
   `plugin_api_v2`, a **single-entry-point ABI**
   (`move_plugin_init_v2(host_api_v1_t*) → plugin_api_v2_t*`, a struct
   of function pointers — `create_instance`, `destroy_instance`,
   `on_midi`, `set_param`, `get_param`, `render_block`), defined
   inline in the plugin's own `.cpp` file rather than a shared header.
   This is simpler to host than `plugin_api_v1`'s shared-memory/`link
   audio.h` contract assumed earlier in this plan — a Push3 host only
   needs to match one struct layout and call one symbol, no shm ring
   buffers required for the DSP-loading part itself.

   A minimal test harness (`dlopen`, build a `host_api_v1_t`, call
   `create_instance`, send a MIDI Note On, call `render_block` in a
   loop) confirmed real, non-silent audio: `peak=4983`, `12696/12800`
   nonzero samples over 50 blocks at 128 frames each, clean
   `destroy_instance` with no crash.

2. **Partially done: block-size flexibility confirmed, RT-scheduling
   still open.** Reran the same harness at **512 frames per block**
   (Push3's actual negotiated Loopback period, after the glitch fix
   above) instead of Move's native 128 — passed identically
   (`peak=4983`, `50846/51200` nonzero). `render_block` is purely
   caller-driven; there is no hard 128-frame assumption in Braids'
   code path. Still open: whether Schwung's documented `SCHED_FIFO 90`
   / core-pinning / ~900µs budget (tuned to Move's SPI interrupt
   cadence) is a real requirement for *any* plugin, or just how
   Schwung's own reference host happens to schedule it — matters less
   now that at least this plugin tolerates a slower, non-Move block
   size, but still worth confirming before committing to a scheduling
   model for the Push3 host.
3. **Build a minimal Push3 host binary** that: opens "Push Hack Virtual
   Audio" for audio in/out (512-frame blocks, per step 2's finding),
   opens Push3's ALSA sequencer for pad/encoder MIDI (reusing
   `internal/midiin`/`midiout`), and loads Braids' `dsp.so` via the
   `plugin_api_v2` contract confirmed in step 1 to process the audio in
   place. Display can come later — this step proves the audio+MIDI+DSP
   chain alone, with no screen output yet.
4. **Add the display path**, once steps 1–3 prove the core loop: draw
   through `push_hook.c`'s `LD_PRELOAD`/shared-memory mechanism (same
   pattern already used for Push3's own display, adapted to a second
   consumer) or via a plain X11 client.
5. **Decide where this host lives.** It is standalone-only (SSH-deployed
   to Push3's own Linux, like every `ableton-push-hack` hack) — it does
   not fit this repo's own module-host architecture, which targets
   **tethered** mode only. The natural home is a new hack in
   `ableton-push-hack/hacks/`, following the same pattern as
   `push-audio-loopback` and `push-display`.

None of these steps carry the same risk profile as the audio
investigation above (no more kernel modules, no more `AT_SECURE`
dead ends expected) — they are ordinary application-level build and
integration work. Step 1's clean pass on the first try is a good sign
for the rest.
