# Coexisting with Live, and taking control of Push

Status: A3, A1, A2, and A5 measured 2026-08-20 — see below,
[docs/protocol/midi-input.md](../docs/protocol/midi-input.md#user-modes-effect-on-routing),
[docs/protocol/usb-and-safety.md](../docs/protocol/usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20),
[docs/platform/macos.md](../docs/platform/macos.md#lives-background-helper-confirmed-2026-08-20),
[docs/protocol/led-output.md](../docs/protocol/led-output.md#led-contention-with-live),
[docs/protocol/display.md](../docs/protocol/display.md#disconnect-while-running),
[docs/protocol/live-handshake.md](../docs/protocol/live-handshake.md).
User Mode confirmed as a working **full** contention workaround — pad input
*and* pad LED output are both exclusively routed by the same mode toggle,
found same-day chasing "can we skip Part B/C"; the `push` helper identified
and its ownership matrix measured; pad-LED stability confirmed steady across
every launch order, no default-on back-off forced by the evidence; no
inbound echo of any kind found, but a recurring handshake-shaped SysEx
pattern is now on record for later decoding. **Part C closed 2026-08-20** —
superseded by the User Mode finding, nothing built, not being pursued
further. **Part B open, unscheduled** — waiting on a Linux tester; B2
already established full ownership is Linux-only. A4 (Windows capture)
tried and **blocked** — the available VM is ARM64 Windows, and USBPcap's
driver cannot load there at all; needs x86/x64 Windows (bare-metal, or an
untested/likely-too-slow QEMU TCG x86_64 VM) to proceed.

## Why

Two halves, one solved and one not.

**The display is already winnable.** Interface-0 exclusivity is
first-claimant-wins and order-independent (`docs/archive/open-questions.md:85-90`),
so launching `pushapp` before Live keeps the screen.

**The pads are not.** CoreMIDI and ALSA are multi-client, so Live holds Push's
MIDI connection regardless of who owns the display. Observed on 2026-08-17 and
confirmed by the operator: with `pushapp` launched last it took the screen, but
Live kept the pad grid — **both its colouring and its MIDI functions**. Pressing
a pad still drove Live. There is no arbitration between the two hosts.

## Two corrections to what the docs currently claim

Both from the operator, and both change the plan's shape.

**1. User Mode's behaviour was never measured.**
`docs/archive/feasibility.md:161-163` states that Push's User Mode "changes MIDI
routing; it does not hand back the display claim", and that sentence is cited
elsewhere as established fact. It is an assumption. Measuring it is now the
first experiment here rather than a late one, because a cheap yes would reshape
the whole question.

**2. Push can no longer be deselected as Live's control surface.** Current Live
ships a background `push` app that manages the Live↔Push connection, and the
preference-panel escape hatch is gone. The "just deselect the control surface"
workaround does not exist. The real question is what that helper owns and
whether it can be stopped independently of Live — and this repo has zero
knowledge of it (no mention of a daemon, launch agent, or helper anywhere in
`docs/`, `plans/`, or the code).

**3. A doc wording bug, which understates the problem.**
`docs/protocol/led-output.md:78-88` says "`pushapp`'s pad-mirror grid started
reflecting Live's Session View colouring". `modules/monitor` renders `padsLit`
(a `map[byte]bool`) in exactly two theme colours and carries no colour
information at all, so that cannot describe the on-screen mirror. It was the
*physical* pads. Rewrite it to say so, and add the MIDI-function half, which is
the more important and currently undocumented fact: the contention is
functional, not cosmetic.

That also pre-answers what was going to be the decisive experiment — Live's LED
writes were never visible to us, so contention detection has no direct
observable signal.

## Part A — measurements

Findings go to `docs/protocol/` and `docs/platform/`, never to `docs/archive/`
(frozen); resolved entries get deleted from `2026-08-18-open-items.md`.

The oracle for "who holds interface 0" is functional, not introspective:
`go run ./cmd/frametest` and read `display.ErrBusy` versus success.

Standing constraints: no blind button sweeps — the only press this whole plan
needs is one named CC 59 with the display already held. No SysEx fuzzing; the
only known-safe device SysEx is read-only command `0x04` (palette lookup). No
interface 6, no control transfers, no firmware operations.

### A3 (first) — User Mode — **done, 2026-08-20, extended same day — bigger than originally scoped**

Result: User Mode is a real, working workaround for **both halves** of
contention, not just pad input. Full writeup:
[docs/protocol/midi-input.md](../docs/protocol/midi-input.md#user-modes-effect-on-routing),
[docs/protocol/led-output.md](../docs/protocol/led-output.md#led-contention-with-live).
Summary — buttons always duplicate to both Live Port and User Port
regardless of mode; pads are exclusively routed, Live Port only with User
Mode off, User Port only with it on, a device-level cutoff confirmed both by
port trace and by watching Live's UI with its generic MIDI-input prefs
disabled; the display claim is untouched by Live launching or by the mode
toggle; Live's outbound LED SysEx never stops; and the mode toggle announces
itself unsolicited on both ports (`0A 01` enter / `0A 00` exit), a usable
signal for C2.

**Same-day follow-up, prompted by "can we just use User Mode instead of
building Part B/C" — the answer turned out to be closer to yes than
expected: pad LED *output* is exclusively routed by the same toggle.**
`tools/ledtest.swift`'s palette sweep to Live Port's output cable renders
nothing while User Mode is on; the identical sweep to User Port's output
cable renders correctly while User Mode is on. So a host that targets User
Port for LED writes can paint its own pad colours *while fully coexisting
with Live* — not just read pad presses. This supersedes the "Live's colours
persist because of a local firmware override" framing above: it isn't an
override masking the grid, Live Port simply stops being the live output
cable the moment User Mode engages, symmetric with input.

**Correction, same day: no code change needed for this at all.**
`internal/midi.OpenRef` already pairs each `PortRef`'s input cable with its
own same-role output cable (`ports.go`'s pairing logic — exact-name match
on macOS/Linux, positional per-unit on Windows), so `pushapp -midi-in
"Ableton Push 3 User Port"` already writes LEDs to the correct cable with
zero changes. Verified end-to-end, User Mode engaged, Live running: the
`monitor` module's own white (palette 120) rendered correctly on press, and
a dedicated single-pad red write (palette 1, unambiguous colour) rendered
correctly at the right grid position. What's still Live-hardcoded by design
and unaffected by this: `Open()`'s zero-arg auto-detect (no selection given,
guesses Live Port on purpose) and `internal/identify.FlashLEDs`'s bare
`OpenOutCable` (no pairing, used only for unit disambiguation). So the gap
between "User Mode is a working manual toggle" and "co-existence with live
pad art works out of the box" isn't a plumbing gap at all — it's purely a
`pushapp-ui` UX gap: nothing in the UI lets a user pick the User Port cable
or explains that they need to.

**UX gap closed, `live-coexistence-output-cable` branch, 2026-08-20.**
`pushapp-ui`'s pairing view only ever listed a unit's Live cable
(`main.ts`'s old `if (!port.isLive) continue`); now every cable — Live,
User, External — is a selectable row, each with role-specific guidance
(Live needs Live closed; User needs User Mode engaged on the device and
coexists with Live running) sourced from the measurements above. Also added
a non-blocking warning when the picked MIDI unit's device model doesn't
match the picked screen's model — a soft safety net, not a hard constraint,
since `plans/2026-08-19-multi-device.md` already established there is no
reliable way to auto-verify a display unit and a MIDI unit are the same
physical box; manual pairing plus Identify stays the real mechanism. No Go
code changed — `internal/midi.OpenRef`'s existing per-role cable pairing
was already correct (see above), this was purely `frontend/`.

**Full end-to-end confirmation, same session, through the real app —
not a raw script.** First attempt hit a real ordering trap worth recording:
opening `pushapp-ui` only lists a unit in the pairing view, it does not
claim anything — Live launched before the operator actually clicked "Pair
and connect" won the display race, same `ErrBusy` as every other
too-late-second-claimant case this whole plan. Corrected sequence, verified
working: Live closed → open `pushapp-ui` → pick screen + **User Port** →
Pair and connect (claims the display) → launch Live → press User on the
device. Result: display stays with `pushapp`, User Mode engages with Live
present, and — bonus, unplanned confirmation of the "PushApp out port" idea
from earlier discussion — **running `modules/seq` exposed its MIDI output
as a port Live could see and record from**, live, while User Mode was
engaged and Live was open the whole time. `pushapp-ui`'s connect hint now
tells the operator to pair before launching Live.

Still genuinely open, not resolved by this: whether User Mode's actual
mode-toggle behavior (not just raw CC59 bytes) requires Live to be running
to engage at all — every test that checked the toggle's real effect had
Live open already. Doesn't block the documented workflow above, since that
workflow always has Live open before the toggle anyway, but is worth an
isolated check if the question ever matters on its own (e.g. a Live-less
session that still wants exclusive pad ownership on a device that's been
left in User Mode from a previous Live session).

Original brief, for the record: 

Because the docs assert an untested assumption, and because a yes is the
cheapest possible answer to everything below.

Hold the display with `pushapp -module monitor`, which also neutralises the
top-row soft buttons. Run `tools/midimon.swift` on *all* Push ports — something
`internal/midi` cannot do, since it opens Live Port only. Press User (CC 59,
`core/push3/buttons.go:104`) three times, to separate real behaviour from
session-dependent behaviour: the undiagnosed duplicate-port observation at
`feasibility.md:963-974` "flips between sessions".

Without Live: does traffic move or duplicate to User Port? Do pads keep arriving
on Live Port? Does MPE channel behaviour change? With Live running: does the
display claim change (the assumption says no — prove or refute); do Live's pad
LEDs go dark; do pad presses stop reaching Live; does `frametest` now succeed
where it returned `ErrBusy`? Any yes is a shipping workaround. A refutation is
equally valuable and must be corrected everywhere it is cited. Also settles the
User/External port-role theory at `2026-08-18-open-items.md:27-33`.

### A1 — the `push` background app: what does it own? — **done, 2026-08-20**

Result: the helper is `Push3.app` (`com.ableton.Push3`), a plain child
process of Live, not `launchd`-managed. Full matrix, all 7 cells measured on
real hardware, in
[docs/protocol/usb-and-safety.md](../docs/protocol/usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20)
and
[docs/platform/macos.md](../docs/platform/macos.md#lives-background-helper-confirmed-2026-08-20).
Headlines: doesn't start without Live (cell 2 closed cleanly); killing it
alone frees the display for ~2.3s before Live's own watchdog respawns it,
not a usable "stop" affordance (cell 4); clean quit releases everything
immediately (cell 5); `kill -9` on Live leaves it orphaned and still holding
interface 0 for ~5.2s before it self-exits via a parent-liveness poll (cell
6) — a real gap in the old "no replug needed" claim, now corrected; launch
order doesn't affect pad contention, only which process keeps the screen
(cell 3 vs. 7, symmetric).

Original brief, for the record: 

Reframed from the dead deselect experiment. The goal is to establish which
process holds interface 0 and which holds the MIDI connection — Live, the
helper, or both — and whether the helper's lifetime is independent of Live's.

Identify it first, snapshotting and diffing at each step:

```
ps -Ao pid,ppid,user,command | grep -iE 'ableton|live|push' | grep -v grep
launchctl list | grep -iE 'ableton|push'
ls -la ~/Library/LaunchAgents /Library/LaunchAgents /Library/LaunchDaemons | grep -iE 'ableton|push'
ls "/Applications/Ableton Live 12 Suite.app/Contents/XPCServices" \
   "/Applications/Ableton Live 12 Suite.app/Contents/Library"
```

Record its real path, parent pid (XPC service of Live, launch agent, or plain
child?), and launch trigger — on login, on Live launch, or on Push plug-in.
That determines whether "quit Live" is even the right lever.

Then the ownership matrix:

1. Cold baseline, no Live, no helper: `frametest` succeeds.
2. Push plugged in, Live *not* launched: does the helper start anyway, and does
   `frametest` already fail? The single most important cell — a yes would mean
   Push is unusable to us without Live ever running.
3. Live running: `frametest` fails as documented. Which process is the claimant?
   Determine by elimination below.
4. **Quit the helper, leave Live running.** Does `frametest` succeed now? Do
   Live's pad LEDs go dark? Does Live still respond to pad presses? Does the
   helper respawn — immediately, on a timer, or not at all? (`launchctl print`
   on its label tells you whether it is `KeepAlive` before you try.) A yes here
   is the workaround that replaces the dead deselect path.
5. Quit Live, leave the helper: `frametest` at +0s, +10s, +60s, plus a `ps`
   check. A surviving helper still holding interface 0 makes
   `usb-and-safety.md:33-34` ("the claim releases when Live quits; no replug
   needed") wrong as written, and is a real product problem.
6. `kill -9` Live instead of a clean quit, then repeat 5. Tests whether release
   depends on orderly shutdown.
7. Both running with `pushapp` holding the display first: does the helper log or
   surface an error, and does Live still get pad input?

Record the exact Live build and macOS version — helper behaviour is
version-specific and this is the kind of finding that silently rots.

**What `pushapp` should then do about the helper is deliberately open.** Cell 4
decides whether a "stop the helper" affordance is even coherent: a `KeepAlive`
agent respawns, so a stop button would be misleading unless it also unloads the
agent, which is a much bigger commitment. Decide after A1. Until then the
supported guidance stays what it is today — don't run `pushapp` with Live open.

**Cell 2 is not a gate on other work.** `pushapp` demonstrably works on this
machine today, so the helper evidently is not claiming the display in the
current setup. Treat it as closing a documentation gap and understanding the
failure mode; the multi-device work
([2026-08-19-multi-device.md](2026-08-19-multi-device.md)) does not depend on any
of this.

### A2 — launch-order matrix, and it must run before C4 lands — **done, 2026-08-20**

Result so far: every cell tested is **steady, not flickering** — no
default-on back-off needed on this evidence, a user toggle suffices. Detail:

- **Live-then-`pushapp`** (both LED flags): display → Live, degrade text
  exactly `display: display interface is claimed by another process (Live?)`
  / `display: continuing MIDI-only — quit Live to get the screen`, inbound
  events arrive fine, pads steady both with default LEDs (both writers
  active) and `-no-leds` (Live only).
- **`pushapp`-then-Live** (both LED flags): display stays with `pushapp`
  (log never updates, no error/degrade), Push3 helper starts normally with
  no error surfaced, inbound events fine, pads steady both ways. Symmetric
  with the previous cell and with A1 cell 7 — order decides the screen only,
  never the pad-LED stability.
- **Control surface set mid-session: N/A.** Dead, same as the deselect
  workaround the plan's correction #2 already flags — there is no live
  toggle to test. The closest equivalent is A1 cell 4 (killing the helper
  mid-session): it respawns in ~2.3s and Live's pad ownership snaps straight
  back.
- **Quit Live with `pushapp` up (default LEDs):** two new findings.
  (1) **`pushapp` never retries the display claim after an initial
  degrade** — confirmed both by the log staying silent after Live quit and
  by code, `internal/display/display.go:51` ("display rather than retry").
  Once degraded, that process stays MIDI-only for its whole run; only a
  relaunch reclaims. The screen fell back to Push's own firmware idle
  screen ("connect Push to a computer"), consistent with nobody holding
  interface 0. (2) **No re-assert step needed**: pads went off on their own
  when Live quit, not stuck on Live's last colours — unlike the User Mode
  case (A3), where Live's colours snap back instantly because Live never
  stopped sending them. `-no-leds` variant skipped as redundant — nothing to
  differ with only one writer ever active.

**Unplug Push with Live present** (`pushapp` holding the display, Live
launched second and degraded): the unplug surfaces as a failed frame write →
`ErrDisconnected` → `host: Push disconnected` logged → clean exit, no crash.
On replug, `pushapp` does not auto-relaunch; Live's still-running background
helper (never itself unplugged) reclaims interface 0 on its own. Full
writeup: [docs/protocol/display.md](../docs/protocol/display.md#disconnect-while-running).

Original brief, for the record: Each cell twice, default then
`-no-leds`: Live-then-`pushapp`; `pushapp`-then-Live; control surface set
mid-session; quit Live with `pushapp` up; unplug Push with Live present.

Per cell: who owns the display; the exact degrade message from `bootstrap.go`;
whether inbound events still arrive; and **what the pads physically do** —
steady last-writer-wins, or flicker at our event rate. Stable means a user
toggle may suffice; flicker means back-off must be default-on. The quit-Live
cell tells us whether back-off needs a re-assert step (it will, if Live's
colours persist on the hardware afterwards).

### A5 — is any of Live's LED output observable? — **done, 2026-08-20**

Both clean negatives, confirmed with `midimon` on all three ports:

- **No inbound NoteOn/CC/PolyAT burst from Session View colour changes
  alone** — 20s capture, actively changing track/clip colours in Live with
  zero physical touches on Push, produced only SysEx (the recurring
  handshake-shaped traffic, see below) and Active Sensing. Zero note/CC
  events.
- **No echo of our own writes.** `tools/ledtest.swift` sent 581 pad/button
  LED-write MIDI messages (Live closed, so no confound) while `midimon`
  captured all three input ports simultaneously — zero NoteOn/CC came back,
  only Active Sensing. Push does not echo `SetPad`/`SetButton` writes on any
  port or channel; the trap this task called out (a press and an echo being
  indistinguishable on the same channel) doesn't apply because there's no
  echo to trip over.

Side finding, written up separately rather than guessed at here: a
recurring SysEx pattern shows up on Live Port continuously whenever Live is
running, independent of presses or writes on either side — looks
handshake/keepalive-shaped but mechanism unconfirmed. Full raw evidence:
[docs/protocol/live-handshake.md](../docs/protocol/live-handshake.md).

Original brief, for the record: the operator's observation already answers the substance. Run the cheap version
anyway: a negative is a durable protocol fact worth writing down, and the
reverse direction is genuinely unknown. With `midimon` on all ports and Live
driving the grid, confirm no inbound `0x90`/`0xB0` bursts appear when Live's
Session View changes. Separately, check whether *our* own `SetPad` comes back to
us — that establishes whether Push echoes at all, independent of Live. If an
echo does exist, note the trap before building on it: a pad press and an LED
echo are the same three bytes on channel 1, so it is only usable if it arrives
on a different port or channel.

### A4 — the host→device capture, never done — **blocked, 2026-08-20**

Tried on a UTM VM running **Windows 11 for ARM64** (the fast, native option
on Apple Silicon). Dead end, not fixable: USBPcap's driver is x86/x64-only
and cannot load on ARM64 Windows at all — surfaces as `USBPcapCMD.exe`
failing with "No filter control devices are available. Failed to query
UpperFilters value size! Code 2", which looks like a driver-install/Secure
Boot problem but isn't; it's an architecture mismatch, and reinstalling
never fixes it. Needs **x86/x64 Windows** specifically — either a UTM VM
running x86_64 Windows (untested; QEMU TCG software emulation on Apple
Silicon, likely too slow for reliable USB capture work) or bare-metal x86/x64
Windows, which the original brief already flagged as the fallback.

Original brief, for the record: needs the Windows 11 VM with USB passthrough (Live has no Linux build, and
CoreMIDI cannot see host→device at all — `feasibility.md:1003-1013`).

Wireshark **with the USBPcap component**; `USBPcapCMD.exe` interactively to find
the root hub and Push's address. Filter
`usb.transfer_type == 0x03 && usb.endpoint_address == 0x03` — exclude endpoint
`0x01` or the 320KB display frames drown everything. Payloads are 4-byte
USB-MIDI packets (`[cable<<4|CIN, b0, b1, b2]`; SysEx uses CIN `0x4`-`0x7`) and
must be reassembled per cable. Expect the `F0 00 21 1D 01 01 <cmd>` prefix
mirroring the device→host commands at `feasibility.md:992-1001`.

Segments, each its own `.pcapng`, kept out of the repo: capture started *before*
Live launches (the handshake — the whole point); Live idle 60s; deliberate state
changes in Live; Live quit; and a control segment of `pushapp` alone on the same
endpoint to calibrate the decoder, without which misparsed cable numbering looks
like a mystery.

Commands present in the launch segment but absent from the idle one are Live's
configuration payload. If it is opaque or looks like a vendor control transfer,
**stop** — we do not replay unknown vendor requests.

Risk: USBPcap forces a device restack, and the hypervisor may present a
synthetic hub. If no Push traffic appears at all, that is why; bare-metal
Windows is the fallback.

Linux `usbmon` cannot see Live, but it is the tool that verifies **our own**
interface-5 writes byte-for-byte in Part B.

## Part B — full-ownership spike (opt-in interface 5) — **open, waiting on a Linux tester, 2026-08-20**

Deprioritized after A3's extended findings (User Mode solves the pad-contention
problem this was aimed at, cross-platform, with no libusb detach needed — see
Sequencing) but deliberately kept open rather than closed: the operator wants
this explored once someone with a Linux machine can help test it, since B2
already established full ownership is Linux-only (macOS refuses outright,
Windows needs a system-wide Zadig rebind that's out of scope). Not scheduled;
resume when a tester is available.

Starts as a throwaway `cmd/` probe, matching the shape of `cmd/probe` /
`cmd/frametest` / `cmd/midiouttest`: each isolates one risky thing, and if the
claim proves impossible the probe deletes with nothing to unwind.

**B1 `cmd/usbmidiprobe`.** All flags off by default. Default: enumerate and
report only — find the USB-MIDI interface by class `0x01`/subclass `0x03`
rather than hardcoding 5 (the same defensive spirit as `findDisplayInterface`),
print alt settings and endpoints, claim nothing. `-claim`: attempt, report the
exact error, stop. `-read`: read `0x83`, decode through `midi.DecodeFor`, which
proves the cable↔port mapping. `-send-one-pad`: a single Note On to `0x03`.

**B2 the detach story — this is what decides the spike.** gousb exposes **no
per-interface detach**: `detachKernelDriver` is private and reachable only via
the forbidden config-wide `SetAutoDetach`. So `usb-and-safety.md:26`'s existing
advice to "detach interface 0 alone" is not currently implementable in Go, and
that line needs correcting regardless of this spike.

- **Linux:** a manual operator sysfs unbind of the MIDI interface only
  (`echo -n '1-2:1.5' | sudo tee /sys/bus/usb/drivers/snd-usb-audio/unbind`,
  reversible with `bind`). Zero code, and it makes the ownership boundary
  explicit. The app never does this itself.
- **macOS:** expect `LIBUSB_ERROR_ACCESS`. The class driver holds the interface
  and there is no legitimate userspace escape. Accept as a documented finding,
  not a blocker.
- **Windows:** would need Zadig to rebind `usbaudio2.sys` to WinUSB —
  system-wide, hard to undo, and it would remove Push's MIDI from every
  application including Live. **Out of scope**, and especially not in the VM,
  which is the A4 capture instrument.

Expected end state: full ownership is **Linux-only**.

**B3 `internal/usbmidi`** (only if B1 succeeds on Linux). Mirrors `midi.Port`'s
surface so the host cannot tell them apart. Decoding goes through the existing
`midi.DecodeFor` after packet reassembly, so the channel-before-CC and
Active-Sensing-before-SysEx ordering rules stay in exactly one place. This
package owns transport and USB-MIDI framing only.

**B4 one interface so `host.Runtime` does not care.** `host.New` has exactly one
caller, so this is cheap. Define `host.Surface` (`Name`, `Device`, `Listen`,
`SetPad`, `SetButton`, `Clear`, `Close`) on the consumer side; `*midi.Port`
satisfies it today unmodified. `bootstrap.Options` gains `ClaimMIDI bool` and
`Open` gains one branch that **degrades to OS MIDI with a log line** on any
failure — the same shape as the existing `ErrBusy` degrade, so `-claim-midi` on
macOS is a clear no-op rather than a crash. Add `-claim-midi` to `cmd/pushapp`;
do **not** expose it in `pushapp-ui`, which has nowhere to explain the
consequences.

**B5 documentation amendment — must land before B1 merges**, because the rule as
currently written forbids the probe. Keep "claim only interface 0 **by
default**" and add interface 5 as an opt-in experimental exception: never
interfaces 1-3, never interface 6, never `SetAutoDetach`, degrade-not-retry on
failure, and the Linux unbind as an operator step the app never performs.

**B6 if Push needs configuration we cannot replicate.** Stop at the first that
works: send nothing (pads already arrive with no handshake, and MPE has been
seen both ways with nothing deliberately changed); replay only what A4 captured
*and we understand*, via a `SendSysEx` added to `internal/usbmidi` only — never
to `internal/midi` — behind its own flag; accept degraded configuration and
document it; never fuzz.

The seam for re-emitting to a DAW already exists (`internal/midiout` plus
`host.Options.OpenMIDIOut`). Nothing to build now.

## Part C — LED contention: detect and back off — **closed, 2026-08-20, superseded**

Not built — `internal/contention`, `LEDPolicy`/`LEDsAuto`, `clearOurs`/
`reassertOurs` never existed in the codebase, confirmed by direct search.
Closed by operator decision rather than left open: User Mode's confirmed
full routing of both pad input and pad LED output away from Live (see A3)
solves the coexistence problem this part existed for, with the display
already non-contentious via first-claimant-wins — there is nothing left
here worth exploring further. C4's real, independent bug (`clearLEDs()`
ungated, blanking the other host's colours on every module switch and
shutdown) is the one piece of this part that could still matter on its own
merits, decoupled from the contention-detection machinery around it — worth
a fresh look later as a plain bug fix if it ever bites, not as part of
reopening this section.

**C1 what is honestly detectable — and it got easier.** Live's MIDI output is
not observable to us, and there is no known "read LED state" command. So there
is no *direct* signal. But **presence detection is now strong rather than
weak**: because Push can no longer be deselected as a control surface, "the
owning process is running" implies "it owns Push's pads". The false-positive
case this design was hedging against does not exist in current Live.

Ranked: **process presence** — Live, or more precisely whichever of Live and the
`push` helper A1 shows to be the real owner — is the primary signal, and it
works in both launch orders. `display.ErrBusy` at claim time is free, already
implemented, and corroborating, but only fires when we launched second, which is
the case the operator's observation shows is *not* the interesting one. Inbound
echo only if A5 turns one up, which is unlikely. User override always works.

Keep the docs honest about the mechanism: we are inferring from a process table,
not observing contention. And keep the override, because the inference is
version-specific and will eventually be wrong.

**C2 `internal/contention`.** No hardware needed. `Status{DisplayHeldByOther,
LivePresent, Processes}` with `SuppressLEDs()` and `Reason()`, plus `Detect` and
a `Watch` polling every 2-5s — never on the MIDI or frame path. Process listing
per OS via `os/exec` (`ps -Ao comm` / `tasklist`), no new dependency, with the
lister injectable so the matching logic is unit-testable. **The process names to
match come from A1 — do not guess them.**

**C3 policy on `host.Options`.** A three-value `LEDPolicy` — `LEDsAuto` (zero
value: on, back off on detected contention), `LEDsOn` (escape hatch), `LEDsOff`
— because those are three genuinely different intents. Keep `NoLEDs` as a
deprecated alias normalised in `New`, rather than renaming a surface both
`cmd/pushapp` and `pushapp-ui` already set. Add `suppressed atomic.Bool` and
`ledsEnabled()`; the two `r.opts.NoLEDs` checks in `moduleHost.SetPad` /
`SetButton` become `!r.ledsEnabled()`. That is the single choke point every Go
and process module already funnels through.

**C4 stop stomping the other host's LEDs.** A real bug, independent of
everything above: `NoLEDs` gates only `moduleHost.SetPad`/`SetButton`, while
`clearLEDs()` is ungated and runs on *every* module switch and on `Shutdown()`,
where `midi.Port.Clear()` brute-forces all 64 pads plus every
`pushmap.LitButtons()` entry to zero. So `pushapp` actively blanks Live's
colouring — including in runs the user explicitly asked to leave LEDs alone.

Track `litPads` / `litButtons` maps of what *we* wrote; add `clearOurs()` (write
0 only where we wrote non-zero) and `reassertOurs()` (replay when suppression
lifts, since the other host's colours persist on the hardware). `clearLEDs()`
becomes `clearOurs()`, which also fixes module switching blanking Live's grid
and `-no-leds` writing 64 zeros. Keep `midi.Port.Clear()` for the genuine
cold-start case in `Port.Close()`, noting that the host no longer uses it
mid-session.

This narrows the "always clear LEDs on every exit path" rule, so amend its
wording to **"clear everything you lit"** — same intent, correctly scoped for a
shared device. Without that amendment the rule and back-off contradict each
other.

**C5 wiring and the affordance.** `bootstrap.Open` already knows whether
`ErrBusy` fired; compute the initial `Status` there, pass `LEDs` through, and log
honestly. Start `contention.Watch` from `Runtime.Run` under `LEDsAuto` only,
routing writes through the existing `portMu`. Add `-leds auto|on|off` to
`cmd/pushapp`, keeping `-no-leds` with a deprecation note. Expose `LEDStatus()`
/ `SetLEDPolicy()` on `PushService` — the same shape as `IsConnected` /
`LastError` — and render a badge with a "drive them anyway" toggle.

## Sequencing

- **A3 before committing to Part B's scope — this fired, 2026-08-20.** User
  Mode doesn't release the display (unaffected either way), but it does
  quiet Live's pad ownership on **both** input and LED output, provided the
  host targets the matching cable — and `internal/midi` already does that
  correctly with zero code changes (`OpenRef`'s existing per-role cable
  pairing). Part B (libusb full ownership, Linux-only) now looks like
  solving a problem User Mode already solves cross-platform, for free —
  deprioritize Part B pending a decision, rather than starting the spike.
  Part C's contention-detection angle turned out not to be needed either —
  closed by operator decision, see Part C above.
- **A4 before B3**, because B6's ladder depends on knowing whether config replay
  is needed.
- **B5's doc amendment before B1 merges.**

Part B is open, unscheduled, waiting on a Linux tester. Part C is closed.

## Doc sync when findings land

`docs/protocol/usb-and-safety.md` (a new "Ableton background processes"
subsection; the opt-in interface-5 exception; the not-implementable
per-interface detach correction), `docs/protocol/led-output.md` (rewritten
contention section — physical LEDs plus the MIDI-function half; the "no inbound
observability" fact), `docs/protocol/midi-input.md` (User/External port roles),
`docs/platform/macos.md` (the helper's name, path, and the commands above),
`docs/platform/windows.md` (the USBPcap recipe),
`docs/platform/linux.md` (the sysfs unbind), a new
`docs/protocol/live-handshake.md`, and `CLAUDE.md`'s known-constraints bullets.
Delete resolved entries from `2026-08-18-open-items.md`. Never touch
`docs/archive/`.
