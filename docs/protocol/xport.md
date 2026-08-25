# xPort (interface 6) — raw observations, not decoded

**Status:** early — passive reads confirm real, structured, unprompted
traffic; byte layout not decoded; a first touch-correlation attempt was
inconclusive (see below) — a real confound in the test method, not a
dead end
**Last verified:** 2026-08-25 (Push 3, macOS, controller mode, Live not
running)
**Authoritative code:** none yet — this is capture evidence only

## Why this exists

`docs/protocol/usb-and-safety.md` has always said "never write to `xPort`
speculatively," but that rule's own origin was never written down — traced
2026-08-25 to this repo's very first commit
(`7dfe0b8`, original `docs/feasibility.md` §8.5), the day `xPort` was first
seen via `cmd/probe`:

> `xPort` (interface 6) — vendor-specific, 2 bulk endpoints, undocumented,
> absent from Push 2's spec. "x" plausibly = XMOS. Purpose unknown. Do not
> send it speculative payloads.

Pure first-principles caution, same day as discovery — not a reaction to
any incident. It was specifically about **writing** (sending payloads);
reading was never tried or separately forbidden. `ableton-push-hack`'s
`push3-internals.md` (from Ableton's GPL kernel source release plus live
SSH inspection, 2026-08-25) independently found that Push 3's external
interface 6 is very likely the *same* interface XMOS exposes internally,
documented there as "Hardware control (LEDs, battery?)" — see
[usb-and-safety.md](usb-and-safety.md)'s `xPort` entry for that reasoning.

## The test

Claimed interface 6 only (never interface 0 — display and screen
untouched throughout), opened its IN endpoint (`0x84`), read with a 3s
timeout per round, 5 rounds, **zero writes** — no OUT transfer, no control
transfer, no `SetAutoDetach`. Interface 6 has no OS class driver bound
(vendor-specific, same as interface 0), so nothing else was using it and
nothing else lost access.

**Result: real, continuous, unprompted data.** Every round returned a full
512-byte packet — this is an active stream, not silence with noise.

## What the bytes look like

Three distinct regions were visible across the ~15s capture window
(nobody was touching the pads or strip during it):

1. **16-bit little-endian value pairs, magnitude ~2500–3300** — e.g.
   `44 0A` (0x0A44=2628), `F8 09` (0x09F8=2552), `C5 09` (0x09C5=2501),
   `6A 0A` (0x0A6A=2666). Right in the range you'd expect from raw ADC
   sensor readings, not from anything MIDI-shaped.
2. **A recurring marker:** the 4 bytes `FF FF FF 3F` (= `0x3FFFFFFF`, a
   suspiciously round sentinel) immediately followed by a single byte that
   **increments across occurrences** — `00`, then later `01`, `02`, `03`,
   `04`, `05`, `06`, `07`, `08`, in order, across the 5 read rounds. Reads
   as a frame/sequence counter inside a structured periodic packet — not
   noise.
3. **Long idle runs:** hundreds of repeats of `00 00 00 81` back to back.
   Plausibly one repeated (status-byte, zero-value) pair per sensor
   channel, all reporting "no touch" — consistent with nobody touching
   the hardware during the capture.

None of this is decoded. The working hypothesis — unconfirmed — is that
this is the same "periodic SysEx heartbeats (~3-5/sec) including LED state
and touch sensor data" `push3-internals.md` documents from the standalone
side, seen here from the wire instead. That would make region 1 raw
capacitive/pressure sensor values and region 3 an idle channel scan; region
2's incrementing counter would be the heartbeat's own sequence number. All
three are plausible reads of the bytes, not proven ones.

## Touch-correlation attempt, 2026-08-25 — inconclusive, real confound found

Tried the "touch a pad while capturing" step below: three sequential
6-round/1s captures (idle, then press-and-hold one pad, then release),
diffed by byte offset. First look was promising — several offsets showed
a suspiciously clean, evenly-spaced pattern (every 4 bytes) with distinct
mean values per phase (idle≈64.5, held≈21.5, released≈0). **Did not hold
up under closer inspection:** the raw per-round values *within* a single
phase are themselves noisy (e.g. one offset read `[0, 0, 129, 129, 129,
0]` across 6 idle-only rounds — the same `0x81` value from region 3 above
cycling in and out on its own). With phases run one after another in
wall-clock time and only 6 rounds each, "value differs between phase 1
and phase 2" is confounded with "value differs because the packet's own
rotating content moved on between phase 1 and phase 2" (region 2's own
incrementing counter already proves this rotation is real) — there's no
way to tell which caused what from three separate time blocks.

**Real methodological lesson, not just a failed attempt:** a valid test
needs rapid interleaved touch/release *within one continuous capture*, so
adjacent rounds are compared at the same point in the packet's own
rotation, not three blocks separated by real time. Not done yet.

## Next steps (not started)

- **Touch-correlation, done properly this time:** one continuous capture,
  alternating touch/release every round or two (not three separate
  time-blocked captures — see the confound found above), so adjacent
  rounds share the same point in the packet's own rotation and only the
  touch state differs between them.
- **Correlate the `FF FF FF 3F <counter>` marker's cadence** against the
  "~3-5/sec" heartbeat rate already documented from the standalone side —
  if the counter increments at a matching rate, that's a strong link
  between this wire-level capture and that on-device finding.
- **Capture for longer** (the 15s window here is short) and look for any
  region that changes with LED state (e.g. light a pad via the existing
  `SetPad` path while capturing xPort) — would test the "LED state" half
  of the heartbeat hypothesis the "touch sensor" half above doesn't cover.
- **Decode the leading region's exact structure** — is it one 16-bit value
  per pad (64 pads × 2 bytes = 128 bytes, doesn't obviously divide the
  observed run cleanly) or something else; needs the full packet boundary
  understood first, which needs a capture longer than one 512-byte read.

## Related

- [usb-and-safety.md](usb-and-safety.md) — the `xPort` rule itself, and why
  writing stays off-limits regardless of this finding
- [live-handshake.md](live-handshake.md) — the other unresolved recurring
  traffic, on the MIDI wire rather than `xPort`, possibly the same
  underlying heartbeat, possibly not
- [midi-input.md](midi-input.md) — MPE's own still-open trigger question,
  a separate investigation from this one
