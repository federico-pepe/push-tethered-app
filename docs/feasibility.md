# Tethered Push 2 / Push 3 controller app — feasibility

**Status:** written 2026-08-08 as a research writeup; stack recommendation (§6)
added 2026-08-09; hardware measurements (§8) 2026-08-09; **working vertical
slice (§9) 2026-08-16**. Sections 1-7 are the original argument and are left as
written — §8 and §9 record what measurement and implementation actually found,
including where §6 was wrong (see §6.1a).
**Question:** could a cross-platform desktop app own a *tethered* Push 2 or Push 3
outright — display, pads, buttons, encoders, LEDs — turning it into a fully
configurable MIDI controller independent of any DAW?

**Answer: yes.** The hard part (the display) is already solved in this repo, it
just doesn't know it yet. What follows is the evidence, the reuse map, and the
ranked list of what's still unverified.

**Scope decisions taken up front (2026-08-08):**
- **DAW-agnostic v1.** No Ableton Live coupling. The app speaks plain MIDI to
  whatever is listening. Live integration is a later, optional layer (§4.5).
- **Both devices first-class from day one.** Push 2 and Push 3-in-controller-mode
  are two devices behind one abstraction, not a port.
- **Audio I/O out of scope for v1** (§4.4).

---

## 1. The load-bearing finding: Push 3's display protocol *is* Push 2's

Everything below rests on this. This repo established it empirically while
building `push-display`, but never wrote it down as a portability claim.

| Property | Push 2 (public spec) | Push 3 (observed here) |
|---|---|---|
| Display endpoint | bulk OUT `0x01` | `0x01` — `hacks/push-display/src/push_hook.c:16` |
| Frame header | 16 B: `FF CC AA 88` + 12 × `00` | identical — `push_hook.c:17` |
| Visible geometry | 960 × 160 px | identical — `core/push3/geometry.go` |
| Row stride | 1024 px (64 px padding) | identical — `push3.Stride` |
| Pixel format | BGR565 LE (b15-11=B, 10-5=G, 4-0=R) | identical — `push_hook.c:18`, `core/display/codec.go:29-32` |
| Line shaping | XOR `0xFFE7F3E7` | `{0xE7,0xF3,0xE7,0xFF}` — `push_hook.c:194` |
| Frames per update | 1 | **2** (duplicated) — `push3.TotalBytes` = 2 × `FrameBytes` |

`push_hook.c:20` already says it out loud: *"Push 2/3: 960×160 px visible, stride
1024 px/row"*.

**The one real delta** was the last row: the Push 3 app writes each frame twice
per update (655360 B vs. 327680 B). **Resolved 2026-08-09 — the duplication is
not required.** A single 327680 B frame renders correctly over USB in controller
mode, sustained at 30fps for 12s with no errors. It is a convention of Ableton's
`Push3` binary, not a demand of the hardware. See §8.

### 1a. MIDI traverses libusb too — bulk ep `0x03` / `0x83`

Second finding, same source, and it matters for the stack choice (§6):
`push_hook.c:534-544` (and the async path at `:585-595`) doesn't just log display
frames — it logs **live MIDI byte dumps on bulk endpoints `0x83` (IN) and `0x03`
(OUT)**. That's real observed traffic with a working byte-dump, not an
assumption.

Consequence: a tethered app can read buttons/pads/encoders and write LEDs over
**the same libusb handle it uses for the display**. No RtMidi, no PortMidi, no OS
MIDI API for talking to the device itself. One dependency instead of three, and
the "two processes fighting over one MIDI input port" problem (WinMM is
exclusive-open) never arises.

Note the standalone device *also* carries MIDI over ALSA seq — that's the
`Push3`-app↔Live path, which is why `push_hook.c` hooks `snd_seq_event_input`
separately. The `0x03`/`0x83` bulk traffic is the app↔hardware path, and that is
the one a tethered host app inherits.

**External references to build against:**
- [`Ableton/push-interface`](https://github.com/Ableton/push-interface) — official
  Push 2 display + MIDI spec. Documents the same header, XOR and geometry.
- [`ffont/push2-python`](https://github.com/ffont/push2-python) — working pyusb
  reference implementation. Useful as a cross-check on interface/altsetting
  numbers and the exact claim sequence, which the spec is thin on.

Two independent sources (Ableton's own docs, and our own on-device capture)
agreeing on header + XOR + geometry is what makes the equivalence claim solid
enough to build on, even before a controller-mode capture (§4.2).

---

## 2. Reuse map

The reason this is tractable: the screen stack in `core/` was written against
plain `image.NRGBA` and raw byte slices, with no device or transport coupling.
That was done for the standalone hacks' benefit, but it means it ports for free.

### Reusable as-is

| Package | What it gives the tethered app |
|---|---|
| `core/gfx` | `FillRect`, `DrawIcon`. Imports only `image`, `image/color`, `image/draw` — verified, stdlib-only. |
| `core/gfx/text` | `DrawText`, `TextWidth`, `Truncate` (basicfont). Only `golang.org/x/image` consumer. |
| `core/gfx/widgets` | `Theme`, `SoftButton`, `DrawBotStrip`, `ListRow`/`ListView`/`RenderList`, `KVRow`/`DrawKVRows`, `DrawBorder`/`DrawMeter`/`DrawArc`, `Knob`. Imports verified as **only** `core/gfx`, `core/gfx/text`, and stdlib `image`/`image/color`/`math`. This is a complete Push-screen UI toolkit and it is already device- and transport-agnostic. |
| `core/display` codec | `ToBGR565` / `FromBGR565` (`core/display/codec.go`). `ToBGR565` produces *exactly* the payload a USB bulk write needs. Its package doc mentions `push_hook.so` — that is naming coupling only; the code touches no shm. |
| `core/push3` | `geometry.go` (display constants), `colors.go` (128-entry LED palette + `ColorByName`), `encoder.go` (`IsEncoderCC`, `DecodeRel`, `ScaleVal`, `ClampInt`). Zero imports. |
| `core/httpx`, `core/sse`, `core/hackcfg` | If the app exposes a local web config UI — which is the pattern every hack in this repo already uses. |

### Not reusable

| Package | Why |
|---|---|
| `core/alsaseq` | Linux `/dev/snd/seq` ioctls. On-device only. A host app uses OS MIDI APIs. |
| `core/display.Shm` | The `push_hook.so` mmap. No tethered analogue — USB replaces it. |
| `core/pmclient` | Client for push-manager's HTTP display API. Standalone-only indirection. |
| all of `hacks/push-display` | The LD_PRELOAD hook exists *because* we can't reach the panel directly on the standalone device. Tethered, we can. The whole hook disappears. |

Note the pleasing inversion: on the standalone device we had to interpose on
Ableton's `libusb_bulk_transfer` calls to reach the screen. Tethered, we *are*
the process making those calls.

### New work

- **USB transport** — `github.com/google/gousb` (cgo/libusb) or raw libusb.
  Claim the vendor interface, write `header || ToBGR565(img)` to ep `0x01`.
  This is the only genuinely new display code, and it is small — on the order of
  100 lines including device enumeration and error handling.
- **Host MIDI** — `gitlab.com/gomidi/midi` (driver-agnostic, has both an RtMidi
  and a portmidi backend) or RtMidi bindings directly. Both devices present as
  USB MIDI class in tethered/controller mode, so this is ordinary cross-platform
  MIDI, no special casing.
- **`core/push2`** — Push 2's button/encoder CC map, mirroring
  `core/push3/buttons.go`. Different physical layout and button complement, but
  identical message conventions: CC ch 0, 127 = press / 0 = release, pad grid
  notes 36–99, relative encoders. Mostly transcription from `push-interface`.
- **Device abstraction** — a small interface over `{button map, pad config,
  capabilities}` so one UI runs on both. Push 3 specifics to model: MPE /
  polyphonic aftertouch (needs a config SysEx to enable), touch-strip behaviour,
  and the transport/layout buttons Push 2 doesn't have.

---

## 3. Proposed `core/` refactor — *proposal only*

Today, device-common facts live in a device-named package, and the display
package assumes exactly one transport. Two changes would make `core/` serve both
worlds:

1. **Geometry is not Push-3-specific.** `core/push3/geometry.go`'s constants are
   Push-2-identical (§1). Either move them to `core/display/geometry.go` and have
   `core/push3` re-export, or duplicate into a new `core/push2`. Prefer the move —
   the comment at `geometry.go:3` already says *"same as Push 2"*.
2. **`core/display` gains a `Writer` notion** — something that accepts
   `ToBGR565` output and puts it on a panel. Existing `Shm` becomes one
   implementation; the tethered USB writer becomes the second. That single seam
   is what would let a Shadow-UI panel render identically on a standalone Push 3
   and a tethered Push 2 with no panel code changed.

**Do not do this yet.** It is only worth the churn if the app is actually built.
Refactoring `core/` for a hypothetical consumer is exactly the kind of
speculative generality `discovery/push-core-refactor.md` was careful to avoid —
that extraction was justified by *three existing* duplicated consumers, not by
an imagined one.

---

## 4. Blockers and unknowns, ranked

### 4.1 USB interface exclusivity — highest risk, and it's a product constraint

Only one process can claim a USB interface. If Live is running with Push selected
as a control surface, **Live owns the display endpoint** and our app cannot have
it. Push's "User mode" changes MIDI routing; it does not hand back the display
claim.

So the DAW-agnostic v1 requires: *deselect Push as a control surface in Live's
preferences (or don't run Live at all)*. That is not a footnote to bury in a
README — it is a defining constraint on what the product is. State it in the
first paragraph of any user-facing docs.

This is also precisely why §4.5's Live integration is hard rather than merely
laborious: the useful version ("show me Live's track names on the screen") is the
one where Live simultaneously wants the screen.

Note this is *interface*-level exclusivity, not device-level — which is what
makes §6.3's co-existence mode possible: claiming only the vendor display
interface leaves Push's separate USB-MIDI-class interface bound to the OS driver
and visible to the DAW.

**Confirmed empirically 2026-08-09 (§8.6).** With Live running and Push as its
control surface, claiming interface 0 fails with `LIBUSB_ERROR_ACCESS` — cleanly,
at claim time, before any write. Everything else stays available: USB
enumeration, all 3 MIDI ports, and 16×16 audio. The claim releases the moment
Live quits, with no replug. So the constraint is real, narrow, detectable and
recoverable — the best shape it could have taken.

### 4.2 Push 3 controller-mode wire path — ~~unverified~~ RESOLVED 2026-08-09

> **Was:** the confirmed protocol was Push 3's *internal* display path, observed
> from inside the device. That the same frames traverse USB-C to a host in
> controller mode was a strong inference, not a measurement. This gated all
> Push 3 transport code.
>
> **Now:** measured directly, and it holds. No bus capture was needed in the
> end — `cmd/probe` read the descriptors and `cmd/frametest` lit the screen.
> Full results in §8.

Resolved without Wireshark. The descriptors gave the endpoints; pushing an
actual frame answered the protocol question more definitively than a packet
dump would have.

### 4.3 Windows driver conflict

libusb on Windows needs a WinUSB-class driver, normally installed with Zadig —
which would **displace Ableton's own driver** and break Live's use of the device.
That's a genuinely bad user experience, not a minor caveat.

Mitigations, in order of preference:
- Check whether the device advertises **WCID / Microsoft OS descriptors**; if it
  does, WinUSB binds automatically to the vendor interface without displacing
  anything.
- Otherwise: ship Windows as display-less (MIDI-only, via ordinary WinMM/WinRT
  MIDI, no libusb) and document the limitation.

macOS and Linux have no equivalent problem — macOS lets libusb claim a
vendor-specific interface with no kext, and Linux only needs a udev rule.

### 4.4 Push 3 audio I/O — out of scope for v1

In controller mode Push 3 presents as a **USB audio class** device, so
CoreAudio / WASAPI / ALSA see its ins and outs with zero work from our app. But
"the app can use Push 3's audio I/O" means *building an audio engine* — device
callbacks, buffer management, a routing model, latency handling. That is a whole
second product.

Recommendation: v1 configures MIDI and drives the screen; the DAW owns audio and
just picks Push 3 as its interface like any other. Revisit only if a concrete
feature demands it.

### 4.5 Live integration — deferred, but note the two paths

Deferred by decision, recorded so the options aren't rediscovered later:

- **Ableton Extensions SDK** (Live 12.2+). A real, supported API. This repo
  already has an `ableton-extension-builder` skill available. Preferred if the
  API surface covers what's needed.
- **MIDI Remote Script + local TCP** — the app runs a listener, a Python remote
  script in Live talks to it. This repo already proves the pattern end-to-end:
  `hacks/browser-bridge` does exactly this on port 7704, including reply-box
  queries for tempo/beat/transport state. Well-trodden, but a Remote Script is an
  install step the user must perform manually (as browser-bridge's own README
  documents at length).

Either way, §4.1 still applies: if Live is driving Push's screen, we aren't.

---

## 5. Phased build sketch

Ordered by risk retired per unit of effort, not by feature value.

- **Phase 0 — verify.** USB capture of Push 3 in controller mode (§4.2). Confirm
  endpoint, header, frame count. Gate for everything Push-3-shaped.
- **Phase 1 — pixels on glass.** Minimal Go binary against a **Push 2**: claim
  interface, render one frame with `core/gfx` + `core/gfx/widgets`, `ToBGR565`,
  XOR, write to ep `0x01`. Proves the entire display stack. Push 2 first here
  precisely because it has none of Phase 0's unknowns.
- **Phase 2 — input.** Host MIDI in/out. Buttons, pads, encoders (`push3.DecodeRel`
  already handles relative encoder deltas). LED feedback via the existing
  128-entry palette in `core/push3/colors.go`.
- **Phase 3 — device abstraction + Push 3.** Write `core/push2`'s map, define the
  capability interface, run the identical UI on both devices.
- **Phase 4 — configurability.** Mapping engine + local web config UI. Reuse
  `core/httpx` / `core/sse`, and lift the mapping model from
  `hacks/push-manager/src/remap.go` — it already models src→out CC/Note with
  absolute scaling and relative-encoder accumulation, which is most of what a
  general controller-mapping engine needs.

---

## 6. Stack recommendation

Requirements as stated: cross-platform, standalone, no additional software
installed on the user's machine, has a UI.

### 6.1 Recommended: Go, single static binary, Wails v3 for UI

| Layer | Choice | Why |
|---|---|---|
| Language | **Go** | `core/` reuse is the entire feasibility argument (§2). Leaving Go throws away a working Push-screen toolkit. |
| USB display | **`github.com/google/gousb`** (cgo → libusb) | Bulk OUT ep `0x01`. `core/display.ToBGR565` already emits the exact payload. ~100 lines. |
| Device MIDI | **depends on the mode — see §6.1a** | libusb in full-ownership mode; an OS MIDI API in co-existence mode. |
| GUI | **Wails v3** | Go backend + embedded SPA in a system webview, single binary. Every hack in this repo already ships an embedded SPA — that frontend work transfers directly. |
| Config / state / live updates | `core/httpx`, `core/sse`, `core/hackcfg` | Reused verbatim. |

### 6.1a Correction: libusb-for-MIDI only works in full-ownership mode

An earlier draft of §6.1 claimed device MIDI could always ride the display's
libusb handle (§1a), collapsing three dependencies into one and sidestepping
Windows' exclusive-open problem. **The §8 measurements show that is only true in
full-ownership mode.** Recording the correction rather than quietly editing the
claim away, because the reasoning matters:

MIDI lives on interface 5, and it is *bound to the OS class driver* — that
binding is precisely why the three CoreMIDI ports exist at all. Claiming
interface 5 with libusb takes it away from CoreMIDI, so the DAW loses Push's
MIDI. That is the definition of full ownership. The two modes therefore need
different input paths:

| Mode | Display | Device MIDI in | To the DAW |
|---|---|---|---|
| Co-existence | libusb, iface 0 | **OS MIDI API** (iface 5 stays with the OS) | DAW talks to Push directly |
| Full ownership | libusb, iface 0 | libusb, iface 5, ep `0x03`/`0x83` | virtual MIDI port |

Consequences, by platform:

- **macOS** — fine either way. CoreMIDI is multi-client, so our app and Live can
  both hold `Ableton Push 3 Live Port` at once. Confirmed in §8.6: all 3 ports
  stayed available with Live running.
- **Linux** — fine. ALSA seq is multi-client.
- **Windows** — **harder.** WinMM is exclusive-open, so in co-existence mode, if
  the DAW holds Push's MIDI, our app cannot read button presses at all. Windows
  MIDI Services fixes this, but that is the same dependency §6.2 already names
  for virtual MIDI output.

Net effect: Windows is now the sticking point for *both* virtual MIDI **out**
(§6.2) and multi-client MIDI **in**. macOS and Linux are unaffected. This does
not block shipping — co-existence mode works today on macOS and Linux, and a
display-only Windows build remains viable — but "one dependency instead of
three" holds only in full-ownership mode, and §6.1 should not be read as
promising it everywhere.

### 6.2 What actually constrains the stack: virtual MIDI out on Windows

Not the GUI toolkit. Once the app owns Push's MIDI in order to *remap* it, it has
to re-emit to the DAW, which means creating a virtual MIDI port:

- **macOS** — CoreMIDI virtual source. ~50 lines of cgo. Fine.
- **Linux** — ALSA seq. **`core/alsaseq` is reusable verbatim here**:
  `CreatePort` with `CapRead|CapSubsRead` is precisely what push-manager already
  does for "Push Manager In". Free.
- **Windows** — no built-in Win32/WinMM API for this. **This is where the "no
  additional software" requirement breaks**, and it is the single hardest
  constraint in the whole project.

Windows options, ranked:
1. **Windows MIDI Services** — Microsoft's new MIDI 2.0 stack, open source,
   includes app-to-app virtual MIDI. The clean answer *if* the target users are
   on a recent enough Windows 11. **Verify current availability and the minimum
   OS build before committing** — do not take the version floor on faith.
2. **teVirtualMIDI** (Tobias Erichsen) — redistributable driver, commercial
   licensing. Known-good, costs money and adds an install step.
3. **Co-existence mode** — §6.3.

### 6.3 The architecture fork this forces

This interacts directly with §4.1's screen-exclusivity constraint and should be
decided alongside it.

**Co-existence mode.** App claims *only* the vendor display interface. Push's
USB-MIDI-class interface stays bound to the OS class driver, so the DAW sees
Push's MIDI ports natively, as it always did. App draws the screen and provides
configuration. No virtual port needed on any platform.
→ Ships on macOS and Linux with genuinely zero extra software.
→ Cost: **no remapping** — the app cannot transform MIDI it does not own.
→ Reads input through the **OS MIDI API, not libusb** (§6.1a), which needs
  multi-client MIDI. Free on macOS/Linux; on Windows, WinMM's exclusive-open
  means button input is unavailable while the DAW holds Push.

**Full-ownership mode.** App claims both interfaces. Remapping, custom modes,
alternate layouts — the actual product. Requires the virtual port, hence §6.2's
Windows problem.

**Recommendation: ship co-existence first.** It retires all the USB risk, proves
the display path end-to-end (§5 Phase 1), and works everywhere while the Windows
MIDI question gets resolved independently.

### 6.4 The tax being accepted

- **cgo means no cross-compilation.** Build matrix on real runners:
  `macos-latest` / `windows-latest` (needs mingw-w64) / `ubuntu-latest`. This
  repo currently cross-compiles everything trivially with
  `GOOS=linux GOARCH=amd64`; that ends here.
- **libusb is LGPL-2.1.** Dynamic-link it, or meet the relinking obligation.
- **Wails on Linux needs `webkit2gtk`** — the one place this stack is not
  actually standalone. Present on most desktop distros, but it is a dependency.

None is a blocker. All three are invisible until the first release, which is
exactly when they are most expensive to discover.

### 6.5 Alternative if the cgo tax is unacceptable: Rust

`nusb` is **pure Rust** — no libusb, no cgo, no LGPL, clean cross-compilation.
With `midir` for host MIDI and `egui` (pure Rust, no system webview, zero deps)
or Tauri for the UI. Genuinely the better engineering.

Costs the `core/` reuse. Porting `gfx` / `gfx/text` / `widgets` is ~2600 lines of
mechanical work against plain pixel buffers — tedious, not hard, and Rust's image
ecosystem is stronger than what would be ported.

**Go if this should work within a month. Rust if it becomes a real distributed
product.** Start with Go.

### 6.6 Rejected

- **Electron** — bundles Chromium, so it technically satisfies "no additional
  software", but ~150 MB for a config panel contradicts the whole low-overhead
  instinct of this project.
- **Fyne / Gio** — pure Go, zero system deps, single binary. A real fallback if
  §6.4's `webkit2gtk` dependency is unacceptable. Uglier, and the embedded-SPA
  work would be rewritten.
- **JUCE / C++** — the industry-standard answer for exactly this product, with
  cross-platform virtual MIDI already solved. Costs all Go reuse plus
  GPL-or-commercial licensing. Only worth it if this ships commercially.

---

## 7. Bottom line

The question was "is this possible". It is, and the specific reason is that
~2600 lines of `core/` turn out to be a device-agnostic Push screen toolkit that
was built for a different purpose. The tethered app is, in its essentials, a
**transport swap**: replace one shared-memory writer with one USB bulk writer.

**Update 2026-08-09 — two of the three unknowns are now closed.** The Push 3
controller-mode wire path (§4.2) and the single-vs-double frame question (§1)
were both settled in one session, on hardware, without a bus capture. Only the
Windows driver question (§4.3) remains open. Measurements in §8.

Stack: **Go + gousb + Wails v3, single binary**, with device MIDI riding the same
libusb handle as the display (§1a, §6.1). Rust + `nusb` is the better engineering
and the right call only if this becomes a distributed product (§6.5).

Two constraints dominate everything else, and both are about *ownership* rather
than capability:

1. **§4.1 — you get the screen or Live does, not both.** Interface-level, so
   co-existence with the DAW's MIDI is still possible (§6.3).
2. **§6.2 — virtual MIDI out on Windows** is where "no additional software"
   breaks. It is the only requirement in this project with no clean answer on all
   three platforms today.

Ship co-existence mode first (§6.3): it works everywhere with zero extra
software, proves the display end-to-end, and defers both constraints instead of
blocking on them.

---

## 8. Measured results — macOS, Push 3 controller mode, 2026-08-09

Hardware: Push 3 in controller mode, USB-C to an Apple Silicon Mac, **Live not
running**. Tools: `cmd/probe` (descriptor dump, read-only) and `cmd/frametest`
(claims interface 0, pushes frames). libusb 1.0.30, gousb v1.1.3.

### 8.1 Device enumeration

`VID 0x2982 / PID 0x1969` (Push 2 is `0x1967`). Composite device
(`bDeviceClass 239` = IAD), USB 2.00, 1 configuration, 7 interfaces:

| # | Name (from `ioreg`) | Class/Sub/Proto | Endpoints |
|---|---|---|---|
| 0 | **Ableton Push 3 Display** | 255/255/255 vendor | `0x01` OUT bulk 512, `0x81` IN bulk 512 |
| 1 | Ableton Push 3 Audio | 1/1/32 audio control | `0x85` IN interrupt 6 |
| 2 | Ableton Push 3 Audio Out | 1/2/32 audio streaming | alt 1: `0x02` OUT isoc 624 |
| 3 | Ableton Push 3 Audio In | 1/2/32 audio streaming | alt 1: `0x82` IN isoc 624 |
| 4 | Ableton Push 3 MIDI | 1/1/0 audio control | — |
| 5 | Ableton Push 3 MIDI | 1/3/0 **MIDI streaming** | `0x03` OUT bulk 512, `0x83` IN bulk 512 |
| 6 | **Ableton Push 3 xPort** | 255/255/255 vendor | `0x04` OUT bulk 512, `0x84` IN bulk 512 |

CoreMIDI exposes 3 ports, named identically to the standalone device's ALSA
ports: `Ableton Push 3 Live Port`, `User Port`, `External Port`.

### 8.2 What this confirms

- **§1 protocol equivalence holds on the wire.** Interface 0's bulk OUT is
  `0x01` — exactly what `push_hook.c` observed internally and what Push 2's
  public spec documents.
- **§1a is now measured, not inferred.** The MIDIStreaming interface uses bulk
  `0x03` OUT / `0x83` IN — *the same endpoint addresses* `push_hook.c:534-544`
  logs inside the standalone device. Device MIDI over the display's libusb
  handle is real.
- **§6.3 co-existence mode is viable.** The display is its own vendor-specific
  interface. Claiming it leaves interfaces 1–5 bound to the OS class drivers,
  so a DAW keeps Push's audio and all 3 MIDI ports.
- **§4.4 was too pessimistic.** Audio is fully class-compliant — macOS binds
  `AppleUSBAudioDevice` with In/Out streams, zero install. Device *access* is
  free; only building an audio engine is work.
- **macOS needs no driver, kext, or elevated privileges** to claim interface 0.

### 8.3 The frame test

Rendered with `core/gfx` + `core/gfx/text` + `core/gfx/widgets` from the sibling
`ableton-push-hack` checkout (via a `replace` directive — cross-repo `core/`
reuse works exactly as that project's CLAUDE.md predicted), encoded with
`core/display.ToBGR565`, XOR-shaped, written to ep `0x01`.

Result: **the screen rendered correctly.** Colour bars in the right order,
widgets legible, full 960×160 extent addressed with no stride bleed.

- **Single frame is sufficient — 327680 B.** The standalone device's frame
  duplication is not a hardware requirement (§1 resolved).
- **`ToBGR565` needs no tethered variant.** Channel order is identical.
- **The screen must be continuously refreshed.** A single frame flashes and is
  then overwritten: with no host driving it, Push redraws its own "connect to a
  computer" idle screen. Holding the display means outrunning the device's own
  renderer.
- **30fps sustained cleanly:** 360 frames in 12.03s = 29.9fps actual,
  9.4 MB/s, zero write errors, no short writes or stalls. USB 2.0 high-speed has
  ample headroom — 60fps ≈ 18.8 MB/s is well within budget.

### 8.4 Gotchas found the hard way

- **Never enable gousb's `SetAutoDetach(true)`.** It is *config-wide*, not
  interface-wide: `Device.Config()` loops over every interface in the
  configuration and detaches each. On this device that would tear audio and MIDI
  away from the OS class drivers — destroying co-existence mode — and it fails
  outright on macOS with `LIBUSB_ERROR_ACCESS`. If Linux ever reports
  `LIBUSB_ERROR_BUSY`, detach interface 0 alone.
- `gousb.Device.Config(n)` only issues `set_configuration` when `n` differs from
  the active config. Ours is already 1, so no disruptive reconfiguration occurs.

### 8.5 Still open

- **§4.3 Windows** — the WinUSB/Zadig driver conflict. Now the only unresolved
  item from the original blocker list.
- **`xPort` (interface 6)** — vendor-specific, 2 bulk endpoints, undocumented,
  absent from Push 2's spec. "x" plausibly = XMOS. Purpose unknown. Do not send
  it speculative payloads.
- **Endpoint `0x81` IN** on the display interface — unused by our write path.
  Possibly status/ack. Never read from.
- **Push 2** — nothing measured on real hardware yet.

### 8.6 Live exclusivity — tested, 2026-08-09

Live 12 Suite running, Push auto-detected as a control surface and showing
Live's UI. Predictions were written down before the run so the results could
falsify them; all four held.

| Test | Result |
|---|---|
| `cmd/probe` (enumeration) | **works** — never opens the device, so descriptors read fine regardless of who claimed what |
| CoreMIDI ports | **all 3 still present** (`Live Port`, `User Port`, `External Port`) — CoreMIDI is multi-client |
| CoreAudio | **`Ableton Push 3 Audio` still available, 16 in / 16 out @ 44.1kHz** |
| `cmd/frametest` (claim interface 0) | **fails: `libusb: bad access [code -3]`** = `LIBUSB_ERROR_ACCESS` |
| Re-claim after quitting Live | **succeeds immediately** — 240 frames @ 29.9fps, no replug or power cycle |

What this establishes:

- **The constraint is exactly interface-scoped.** With Live owning the display,
  a third-party app still gets USB enumeration, all MIDI, and 16×16 audio. It
  loses the screen and nothing else. §6.3's co-existence mode is measured, not
  assumed.
- **Failure is clean and detectable.** The error arrives at claim time, before
  any bulk write. An app can catch it, tell the user "Live currently owns the
  display", and run in a degraded mode rather than crash or corrupt state.
- **The lock releases properly.** No sticky claim, no replug needed. Worth
  having tested deliberately: an exclusivity constraint that failed to release
  would have been a far worse problem than the constraint itself.
- **16 audio channels each way** is more than Push 3's analog I/O count.
  Probably internal routing buses. Not investigated.

### 8.7 MIDI input — measured 2026-08-09 (Live closed)

Captured with `tools/midimon.swift` via **CoreMIDI**, i.e. the co-existence
input path (§6.1a), not libusb. Two runs: a broad one (pads/buttons/encoder/
strip) and a targeted encoder-only one to disambiguate.

**Push emits MIDI with no host handshake.** Live was closed; the device sends
immediately on connection. Nothing has to "activate" it.

**All device events arrive on `Ableton Push 3 Live Port`.** `User Port` and
`External Port` carried only keepalive.

#### MPE is enabled by default

Pad note-ons rotate across **channels 2-16** while channel 1 stays global —
textbook MPE channel rotation. Per-note expression is live out of the box:
channel pressure (0-127) and CC 74 slide arrive on each note's own member
channel, and pitch bend (range seen 192-14528) provides per-note glide.

**The disambiguation rule — channel 1 is the control surface, channels 2-16 are
per-note MPE:**

| CC | Channels observed |
|---|---|
| 20, 21, 22, 104-107 (buttons) | ch1 only |
| 71 (encoder 1) | ch1 only |
| 74 (MPE slide) | ch2-16 only, never ch1 |

This matters because Push 2 assigns **CC 71-79 to its nine encoders**, and CC 71
/ CC 74 are *also* MPE's standard timbre controllers. The numbers collide; the
channel does not. Any input decoder must branch on channel first, then CC —
treating CC 74 as "encoder 4" without checking the channel would turn pad slide
into phantom encoder movement.

#### Confirmed control-surface map (Push 2 conventions carry over)

- **Pads:** 8×8, notes **36 (bottom-left) to 99 (top-right)**. Verified at both
  corners.
- **Encoders:** relative, two's-complement — `val 1` = +1 click, `val 127` = -1.
  Encoder 1 = **CC 71** (Push 2 uses CC 71-79 for its nine encoders).
  `core/push3.DecodeRel` decodes this unchanged.
- **Encoder touch:** `Note On ch1 note 0..10`, vel 127 = contact, Note Off =
  release. Encoder 1 = note 0. Matches Push 2.
- **Touch strip touch:** `ch1 note 12`. Matches Push 2.
- **Buttons:** CC, `127` = press, `0` = release. `CC 104-107` = row above the
  screen, `CC 20-22` = row below.

#### Gotcha: Active Sensing

Push emits `0xFE` at roughly **37 messages/second** as a keepalive — over half
of all traffic in the broad capture (1117 of 2233 messages). System realtime
(`0xF8`-`0xFF`) must be filtered before any decode, and must be tested *before*
masking with `0xF0`, or `0xFE` is misread as SysEx. `tools/midimon.swift` hit
exactly that bug on the first run.

#### Still unmeasured

Full button CC map (only 7 buttons were pressed), the remaining encoders,
whether MPE can be switched off via SysEx, and the `External Port` / `User Port`
roles.

### 8.8 Touch-sensor map correction — measured 2026-08-09

`core/push3`'s touch-sensor note numbers are wrong. Found by replaying captures
through `cmd/mapcheck`, which cross-references live MIDI against the shared
constants and flags anything unaccounted for. Confirmed with a 60s capture
touching each sensor in a known order, no turning, so sequence alone identifies
each note.

| Sensor | Measured | `core/push3` | |
|---|---|---|---|
| Encoders 1-8 | **0-7** | 1-8 | off by one |
| Volume wheel | **8** | 9 | off by one |
| *(note 9)* | *unused* | — | gap |
| Tempo wheel | **10** | 10 | correct |
| Jog wheel | **11** | 11 | correct |
| Touch strip | **12** | *absent* | undocumented |
| D-Pad center | **13** | 13 | correct |

Not a uniform shift: notes 0-8 cover the eight encoders plus the volume wheel,
note 9 is unused, and everything from 10 up was already right. The old
numbering looks like it assumed a contiguous 1..10 run for the encoders and both
wheels — and the unused note 9 is exactly what that assumption would paper over.
The upstream doc claims empirical verification, which suggests the range was
extrapolated from one or two measured endpoints rather than swept.

**Also wrong: the encoder direction prose.** `core/push3/buttons.go:7` and
`docs/push3-button-map.md` both state *"CW=127, CCW=1"*. Measurement says the
opposite — turning clockwise sends `1`, counter-clockwise sends `127` — which
agrees with `DecodeRel`'s implementation (`1..63` positive, `127` = -1) and with
Push 2's published spec. The **code is correct; only the prose is inverted** in
both places.

**Undocumented behaviour: the encoders accelerate.** Deltas of `+8` and `-11`
appear on fast turns, so a consumer must use the decoded signed value rather
than treating each message as a single click.

#### Where the fix lives

Deliberately **not** fixed in `core/push3`. That module is shared with
`ableton-push-hack`, whose standalone hacks were built against the current
values, and whose map doc claims its own empirical verification. Changing shared
constants on the strength of a tethered-only measurement risks breaking working
hacks to fix a map that may never have been exercised.

The correction lives in `internal/pushmap` instead, which documents the
divergence and overrides only the touch notes — `core/push3` remains
authoritative for pads, button CCs, encoder CCs, the LED palette and
`DecodeRel`. `internal/pushmap`'s test suite includes `TestDivergesFromCore`,
which **fails if `core/push3` is ever corrected upstream** — turning the
duplication into a self-clearing reminder rather than a silent fork.

If the standalone device is ever re-measured and agrees, fold `pushmap` into
`core/push3` and delete it.

### 8.9 LED output — measured 2026-08-09

Driven with `tools/ledtest.swift` over **CoreMIDI**, i.e. the co-existence
output path (§6.1a). 581 messages, ~8ms spacing, no drops or coalescing.

**Confirmed:**

- **Pad LEDs work as `core/push3/colors.go` documents.** Note On ch1,
  note 36-99, velocity = palette index, `0` = off. All 64 pads lit with
  distinct colours in a single sweep.
- **Pad geometry confirmed from the output side.** A row-by-row walk starting
  at note 36 travelled **bottom to top**, so note 36 is bottom-left. This had
  only ever been measured from the input side (§8.7); output and input now agree
  independently, and `push3.PadNote`/`PadCoord` are correct as written.
- **No handshake needed for output either.** Push accepts LED commands with Live
  closed, same as it emits input without one (§8.7).
- **LED output works in co-existence mode.** Nothing beyond interface 0 is
  claimed; the LED traffic rides CoreMIDI to the `Live Port` while the MIDI
  interface stays bound to the OS class driver.

**Not separately verified in this run:** button-LED brightness (step 4) and
exact palette-index-to-colour fidelity (step 5) were sent and drew no errors,
but only the pad sweep and row order were visually confirmed. Treat the
brightness ramp as plausible, not measured.

**Practical note:** `ledtest` clears every LED on all exit paths including
SIGINT. A probe that leaves the device lit makes the next run ambiguous — you
cannot tell a fresh result from a stale one.

#### What this completes

Display out (§8.3), MIDI in (§8.7) and LED out (§8.9) are now all confirmed
working **simultaneously in co-existence mode on macOS, with zero additional
software installed**. That is the whole v1 product surface, minus remapping,
demonstrated end to end on real hardware.

---

## 9. Vertical slice — working app, 2026-08-16

`cmd/pushapp`: one Go binary that claims the display, reads the control surface
and drives the LEDs. Built to prove the **stack**, not the protocol — §8 had
already confirmed every path, but across three languages (display in Go, MIDI in
and LED out in Swift probes), with no Go route to OS MIDI at all.

**Measured:** 726 frames in 24.3s = **29.9fps**, 272 MIDI events, 137 pad
presses, in a single process. Screen showed a live 8×8 mirror of held pads,
encoder accumulators and an event log; physical pads lit white on press and
cleared on release. Input decode, render and LED output round-trip confirmed.

### 9.1 The Go MIDI question is settled

`gitlab.com/gomidi/midi/v2` with `drivers/rtmididrv` **vendors the RtMidi C++
sources** (`RtMidi.cpp`/`RtMidi.h` ship inside the module), so cgo compiles them
in and there is **no system package to install** — no brew rtmidi, no portmidi.
One dependency covers all three platforms, and the cgo tax was already paid for
libusb. This removes the last unknown from §6.1's stack recommendation.

### 9.2 Structure

The probes' logic was promoted into packages rather than left in `cmd/`:

- `internal/display` — claims interface 0, owns the frame header, XOR shaping
  and a reused 320KB buffer so the refresh loop does not allocate per frame.
  Exports `ErrBusy` for the Live-owns-the-screen case.
- `internal/midi` — OS MIDI in/out with a decoder that **branches on channel
  before CC**, per §8.7. Pads, buttons, encoders, touch sensors and MPE
  expression become typed events.
- `internal/pushmap` — now also holds the shared CC/touch name tables, so
  `cmd/mapcheck` and `cmd/pushapp` cannot drift apart.

### 9.3 Not yet verified

- **The `ErrBusy` degrade path is implemented but untested.** Live was closed
  for this run, so the "Live owns the display, continue MIDI-only" branch has
  never actually executed. Worth running once with Live open.
- **Sustained-load behaviour.** 24s at 30fps is not an endurance test; nothing
  is known about drift, leaks or thermal behaviour over hours.
- `ToBGR565` uses `img.At()` per pixel (153,600 interface calls per frame). It
  holds 30fps comfortably today, but it is the obvious first bottleneck if
  60fps or a busier UI is wanted.

### 9.4 Screen capture, and two bugs it caught

`cmd/pushapp -capture out.mp4` records what the app draws. Frames are tapped
from the render path before the USB encode, so recording adds no USB traffic and
cannot disturb the panel. `.mp4`/`.mov` pipe raw RGBA to ffmpeg; `.gif` is
encoded in-process with no external tool.

**Panel-accurate by default.** Push's panel is BGR565, so the RGBA we render is
not what the hardware shows. Each frame is round-tripped through
`ToBGR565`/`FromBGR565` before recording, giving the recording the same colour
banding the panel has. `-capture-raw` records the source image instead — it
looks better, but it is not what you saw.

Verified: 391 frames, 960×160 h264, 13.07s, 97KB.

**Looking at a captured frame immediately found two bugs that the logs did not:**

1. **The jog wheel decoded as a button.** `push3.IsEncoderCC` covers CC 71-79
   and 14 but **omits the jog wheel at CC 70**, even though `core/push3`'s own
   map documents it as relative. Jog turns fell through to the button branch and,
   since both `1` and `127` are non-zero, produced an endless stream of "button
   presses" while every encoder counter stayed at 0. Fixed with
   `pushmap.IsRelativeEncoderCC`, which extends the upstream predicate rather
   than editing it (same containment strategy as §8.8).

2. **Non-ASCII text renders as tofu.** `core/gfx/text` uses
   `basicfont.Face7x13`, which is ASCII-only — an em-dash in the title bar drew
   as a missing-glyph box. **Anything drawn to Push's screen must be ASCII.**

Both were invisible in the terminal output, which reported healthy frame rates
and event counts throughout. Worth remembering: for a device whose entire output
is a screen, looking at the screen is a distinct verification step.

### 9.5 Contradiction to resolve: MPE may not always be on

§8.7 records "MPE is on by default — pad note-ons rotate across channels 2-16",
measured 2026-08-09. On 2026-08-16 a capture showed pad events arriving on
**channel 1** instead, in the same co-existence setup with Live closed.

Both observations are real; nothing in between was deliberately changed. The
device was disconnected and reconnected between sessions. Possible explanations:
a Push-side mode that persists across sessions, a power-cycle default, or
something about the order in which ports are opened.

**Unresolved.** The decoder handles pads on channel 1 and 2-16 alike, so nothing
is broken — but "MPE is on by default" should be treated as *sometimes true*
until the trigger is identified. Do not build a mapping model that assumes
either layout.

---

## 10. Push 2 on hardware — 2026-08-16

First Push 2 measurement in the project. It had been a stated day-one goal since
2026-08-08 with nothing behind it; the button map was explicitly "unwritten".

**Result: `cmd/pushapp` ran on Push 2 unmodified** — claimed the display,
rendered the same UI at 29.9fps, decoded pads and buttons, and lit pad LEDs.
Confirmed visually. Two small changes were needed first, both device-agnostic
rather than Push-2-specific (§10.3).

### 10.1 Descriptors

`VID 0x2982 / PID 0x1967`, USB 2.00, `bDeviceClass 0` (not a composite IAD
device like Push 3), bus-powered at 500mA, **3 interfaces**:

| # | Class/Sub | Endpoints |
|---|---|---|
| 0 | **255/255 vendor — display** | **`0x01` OUT bulk 512**, `0x81` IN bulk 512 |
| 1 | 1/1 audio control | — |
| 2 | 1/3 MIDIStreaming | `0x02` OUT bulk 512, `0x82` IN bulk 512 |

### 10.2 Identical vs different

**Identical to Push 3 — no abstraction needed:**

- **The display interface, exactly.** Interface 0, vendor-specific 255/255/255,
  bulk OUT `0x01`, 512-byte packets. Same frame header, same XOR, same
  960×160/stride-1024 BGR565. §1's protocol-equivalence claim now has hardware
  on both sides rather than one device plus a spec.
- **Pad grid.** Notes 36-99, note 99 = top-right, decoded as (8,8) by
  `push3.PadCoord` with no change.
- **LED protocol and palette.** Note On + palette index; index 124 is white on
  both devices.

**Different:**

| | Push 2 | Push 3 |
|---|---|---|
| Interfaces | 3 | 7 |
| MIDI endpoints | `0x02`/`0x82` | `0x03`/`0x83` |
| MIDI ports | 2 (Live, User) | 3 (+External) |
| Audio | none | class-compliant 16×16 |
| `xPort` | absent | present |
| Power | bus, 500mA | self, 0mA |
| MPE | **no** — pads on ch1 | yes (usually — §9.5) |
| Vendor interfaces | **1** | 2 (display + xPort) |

The MIDI endpoint difference matters only for full-ownership mode; co-existence
goes through the OS, which is why the app did not care.

Push 2 having exactly **one** vendor-specific interface also means
`findDisplayInterface`'s single-candidate path applies, so it cannot pick wrong
even if the interface string is unreadable.

### 10.3 What had to change (and what did not)

- **MIDI port discovery.** `internal/midi` hardcoded
  `"Ableton Push 3 Live Port"`. Now matches any port containing "Push" and
  "Live Port", so one build finds either device.
- **Display interface discovery** (already done earlier that day for other
  reasons) — the hardcoded interface 0 was a Push 3 assumption that happened to
  be right here.
- **Nothing else.** No display variant, no palette variant, no geometry change.

### 10.4 Button map: partially divergent

Two buttons appeared in a short session:

- `CC 110` decoded as "Device View" — matching Push 3's map.
- `CC 111` came through as **unmapped** — a Push 2 control with no Push 3
  equivalent.

So the maps overlap but are not the same, confirming a `core/push2`-style table
is genuinely needed. The device abstraction, though, is **much smaller than §2
assumed**: display, pads, LEDs and encoders are shared, and only the button CC
table and MPE behaviour differ. `cmd/mapcheck` can sweep Push 2's map the same
way it swept Push 3's.

### 10.5 Consequence for the product decision

The 2026-08-08 scope decision "both devices first-class from day one" is, for
the core I/O paths, **already true** — accidentally, because the display stack
was built device-agnostic from the start. Push 2 support is now a button-map
exercise rather than a port, which strengthens options B and C in
`plans/2026-08-16-product-shape-decision.md`: whatever gets built reaches two
devices, and Push 2 units are cheap and plentiful compared with Push 3.

### 10.6 Push 2 button map — swept 2026-08-16

Two ordered sweeps through `tools/midimon.swift` → `cmd/mapcheck`, pressing in a
known sequence with pauses so identity comes from order rather than guesswork.
**Coverage: 75/80 CC, 12/14 touch notes, zero unknowns.**

**Most of Push 2 matches `core/push3`'s CC table exactly** — both screen rows
(102-109, 20-27), the scene column (36-43, CC 36 at the *bottom*), transport,
modes, views, Shift/Select, octave and page, and encoders 1-8 at CC 71-78.

Five controls differ, now in `internal/pushmap/push2.go` as deltas:

| CC | Push 2 | Push 3 |
|---|---|---|
| 15 | **Swing encoder** | no such control |
| 52 | **Master** | uses CC 28 "Select (main)" |
| 53 | **Stop Clip** | uses CC 29 "Stop Clips" |
| 87 | **New** | uses CC 92 |
| 111 | **Browse** | no such control |

Push 3-only, absent on Push 2: the jog wheel (CC 70, 93-95), D-Pad centre (91),
and Set/Help/Save/Lock (80-83).

#### Note 9 — the two devices explain each other

Push 2's touch notes run **contiguously 0-10**: encoders 1-8 = 0-7, master
volume = 8, **Swing = 9**, Tempo = 10, touch strip = 12.

Push 3 dropped the Swing encoder and left **note 9 unassigned** — which is
exactly the hole found in §8.8, and exactly what makes the upstream "encoders
are notes 1-8" numbering look plausible. The likeliest story is that the Push 3
map was extrapolated from Push 2's contiguous run, shifted by one, and never
swept per-control.

This also independently re-confirms §8.8's correction: Push 2 puts encoder
touches on **notes 0-7**, matching `internal/pushmap` and contradicting
`core/push3` on a second, different device.

#### Unresolved: arrow-key CCs

Pressed as up, down, left, right, the arrows produced CC `46, 45, 44, 47`.
Push 3's map says Up=46, **Right=45**, **Down=47**, Left=44 — so either Push 2
differs on down/right, or the press order deviated from the instruction. A later
group in the same capture did arrive out of the requested order, so the sequence
is not trustworthy here. **Treat Push 2's down/right as unverified**; one
targeted capture pressing only Down settles it.

#### Tooling

`cmd/mapcheck` now **auto-detects the device** from the port name that appears in
every `midimon` line, and annotates with that device's table — no flag needed.
`internal/midi` decodes per-device too, since which CCs count as encoders differs
(Push 2 adds CC 15, and has no jog wheel at CC 70).
