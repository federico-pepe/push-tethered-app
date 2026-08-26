# xPort (interface 6) — raw observations, not decoded

**Status:** touch correlation **confirmed**, per-pad, 2026-08-26 — two
different pads lit up two different byte offsets (105 and 9) in a 136-byte
marker-aligned frame, each with 100% agreement across 10 independent
toggles; full offset→pad map and pressure-scaling still undecoded
**Last verified:** 2026-08-26 (Push 3, macOS, controller mode, Live not
running)
**Authoritative code:** [cmd/xporttest](../../cmd/xporttest) — read-only,
never writes to xPort

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

## Second attempt, 2026-08-26 — also inconclusive, a tooling bug this time

Built `cmd/xporttest` to do the continuous-capture version the first
attempt's lesson called for: fixed read-count loop, `time.Sleep` between
reads, phase toggled by round count. Two runs (200 rounds, then again with
a wider per-segment margin to absorb reaction lag) both came back with zero
offsets surviving the consistency check.

Root cause, found by dumping raw reads and locating every `FF FF FF 3F`
marker occurrence with its byte offset: the marker recurs every **136
bytes** almost everywhere, but with periodic gaps up to ~1.5KB. The 136-byte
gap is the real frame period — three to four frames land inside every
512-byte read. The large gaps are `time.Sleep(period)` between reads
silently losing everything the device streamed out during the sleep, on
top of reads never being frame-aligned in the first place (a fixed 512-byte
read boundary has no reason to land on a 136-byte cycle boundary). Neither
of the first two attempts' "per-offset in a 512-byte read" comparisons
could ever have worked: offset *O* in one read and offset *O* in the next
aren't the same field once reads aren't frame-aligned and data goes
missing between them.

## Touch-correlation confirmed, 2026-08-26

Rewrote `cmd/xporttest` to fix both problems: reads happen back-to-back
with no sleep (nothing emitted between reads is lost), each read is
timestamped, and touch/release prompts fire on a wall-clock ticker (2s
per phase) independent of read count. Analysis then concatenates every
read into one continuous byte stream, finds every `FF FF FF 3F` marker
occurrence, infers the true frame length from the most common marker-to-
marker gap (136 bytes — matches the raw-dump finding above, now systematic
instead of eyeballed), and slices the stream into marker-aligned frames.
Only then does the same per-offset, per-toggle consistency check the first
attempt's lesson called for — each surviving offset has to separate touch
from release in *every one* of the individual toggles, not just in
aggregate.

One 20s run, 2s phases (10 toggles: 5 touch, 5 release), 400ms margin
discarded after each phase change for reaction lag: **63,469 marker-aligned
frames survived**, and byte **offset 105 of the 136-byte frame** separated
touch (mean 5.5) from release (mean 0.0) with **100% agreement across all
10 segments** — not a rotating-content artifact, since it held on every
single toggle independently, and not noise, since release was a clean 0
throughout.

**Confirmed per-pad, same day:** a second 20s run touching a *different*
pad lit up **offset 9**, not 105 — mean touch 6.5 vs release 0.0, again
100% agreement across all 10 segments. Two different pads, two different
offsets, same clean binary-looking jump. This rules out a global "any pad
touched" flag: the frame really does carry per-pad addressing, at least
for these two pads.

**What's still open:** the full offset→pad map (2 of presumably up to 64
pads placed so far — 105 and 9 don't obviously suggest a grid stride yet,
need more points), whether the value scales with pressure or is a plain
touch/no-touch flag (5.5 and 6.5 seen so far, both look like they could be
noise around a threshold rather than two meaningfully different levels —
no intermediate values recorded yet either way), and whether every pad
gets a full byte (136 bytes would only cover 136 pads at 1 byte each, or
68 at 2 bytes — either fits 64 pads with room to spare for the marker,
counter, and other fields already seen).

**Status: parked as a stretch goal, 2026-08-26.** A third run (a bottom-row
pad) found nothing — not even a marker/frame-extraction failure, just zero
offsets surviving the consistency check, unlike the first two pads' clean
100%-agreement hits. Unresolved whether that's a real edge-of-grid
difference, a mistimed touch relative to the 2s phase windows, or something
else — not investigated further. Fully mapping and decoding xPort would
take a lot more of this one-pad-at-a-time probing for a payoff (richer
per-pad pressure data) that's speculative and not needed by anything
currently planned. Revisit if a concrete module need shows up for
finer-grained touch data than MIDI already gives.

## Next steps (not started)

- **Build the offset→pad map** — repeat the same test methodically, one
  pad at a time (e.g. all 8 pads in one row, or the 4 corners first) and
  record which offset lights up for each; two points (105, 9) aren't
  enough yet to guess whether it's row-major, column-major, or something
  else.
- **Check whether the value scales with pressure** — press lightly vs hard
  during the touch phase and see if the corresponding offset varies
  continuously rather than jumping straight to a fixed value (5.5, 6.5 so
  far — close enough to each other that this still isn't confirmed either
  way).
- **Correlate the marker's cadence** against the "~3-5/sec" heartbeat rate
  documented from the standalone side (`push3-internals.md`) — at 136
  bytes/frame and whatever the observed frame rate turns out to be, check
  whether it lines up.
- **Test the LED-state half of the heartbeat hypothesis** — light a pad
  via the existing `SetPad` path while capturing xPort, same marker-aligned
  frame approach, and look for a byte that tracks LED state instead of
  touch.

## Related

- [usb-and-safety.md](usb-and-safety.md) — the `xPort` rule itself, and why
  writing stays off-limits regardless of this finding
- [live-handshake.md](live-handshake.md) — the other unresolved recurring
  traffic, on the MIDI wire rather than `xPort`, possibly the same
  underlying heartbeat, possibly not
- [midi-input.md](midi-input.md) — MPE's own still-open trigger question,
  a separate investigation from this one
