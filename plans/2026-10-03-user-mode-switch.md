# Switch Push into User Mode from the host

**Status:** built and tested on macOS (Push 3) with `pushapp -keep-user-mode`. The `pushapp-ui` checkbox, the manual and the docs are written, and the UI is built but not yet tried by hand. Windows not tested.
**Date:** 2026-10-03

## Goal

Let PTA put a Push into User Mode by itself. Today the user must start Live,
press User, quit Live, start PTA, start Live again and press User again. With a
host command, PTA could start first, claim the screen, switch the Push, and
only then let Live start.

## What we know

- Push announces a mode change by itself, on Live Port and User Port at the
  same time, when the user presses User. Enter is `F0 00 21 1D 01 01 0A 01 F7`.
  Exit is `F0 00 21 1D 01 01 0A 00 F7`. See
  [midi-input.md](../docs/protocol/midi-input.md) (measured 2026-08-20).
- The Push 3 helper inside Live 12 (`Helpers/Push3.app`, Python modules
  `Push2/user_component` and `Push2/sysex`) has a `UserComponent` with
  `toggle_mode` and `force_send_mode`. It builds the message with
  `MODE_SWITCH_MESSAGE_ID = 10` (0x0A), `LIVE_MODE = 0` and `USER_MODE = 1`,
  after the prefix `00 21 1D 01 01`. So the host sends the same bytes that the
  Push announces. Read on 2026-10-03 from identifiers and constants only.
- `push-manager` in `ableton-push-hack` uses the same prefix
  `F0 00 21 1D 01 01` for its Push 3 SysEx.
- The user's own Windows steps (switch with Live, quit Live, start PTA, start
  Live, press User again) suggest that Live puts the Push back into Live Mode
  when it starts. Not measured.

## Results (macOS, Push 3, 2026-10-03)

How we measured. With Live and PTA closed, nothing lights the Push, so User
Mode and Live Mode look the same. The visible proof is the cable that pad
presses use: Live Port in Live Mode, User Port in User Mode. The tool
`cmd/usermodetest` logs that, and it logs every mode announcement. Tests A and
B in the first form of this plan (judged by LEDs) proved nothing.

1. **The Push accepts the switch from the host.** `F0 00 21 1D 01 01 0A 01 F7`
   sent with no User press: the Push announced `User` on Live Port and User
   Port within 1 ms, and the next pad presses arrived on the User Port.
   `... 0A 00 F7` brought it back.
2. **The announcement is the answer to a switch.** The Push does not switch on
   its own when the user presses User. With nothing sending, a User press
   changed nothing: no announcement, and pads stayed on the Live Port. The host
   decides. A press only sends CC 59 to the host, on Live Port and User Port.
   This is why Live must be running today.
3. **Cables.** The Live Port output and the User Port output both work. The
   External Port output does not (no announcement).
4. **Live resets the mode when it starts.** With the Push in User Mode, opening
   Live put it back into Live Mode within a second. The log of the Live start:
   identity reply (`F0 7E ...`), a reply with command `0x42`, a reply with
   command `0x3E`, then the announcement `mode Live` 0.7 ms later, because Live
   sent `0A 00`. `0x3E` is `GET_HOST_MODE` in Live's helper.
5. **Pad presses follow the mode at once.** After each announcement the pads
   used the cable of the new mode.

## Safety

Read [usb-and-safety.md](../docs/protocol/usb-and-safety.md). This work only
sends ordinary MIDI SysEx on a MIDI output cable.

- Send only command `0x0A` with value `0` or `1`. Do not send `0x39` (power)
  or `0x3D` (host mode). They exist in Live's helper and we do not understand
  their effect.
- `cmd/usermodetest` can send these two messages only. It refuses a port that
  is not a Push.
- Pressing User on the Push undoes any result.

## Device tests

Setup for every test: quit Live and `pushapp-ui`. `go run ./cmd/usermodetest -list`
shows the port numbers. On the Mac the Push 3 ports are `#0` Live, `#1` User,
`#2` External.

| # | Test | Result |
|---|---|---|
| A | Send User mode to a cable with `-out N -mode user`. Watch pads with `-watch`. | Live Port and User Port: works. External Port: no |
| B | The User press alone, with nothing sending. | No mode change |
| C | Push in User Mode, then start Live. | Live puts it back into Live Mode |
| D | Listen on all cables while sending. | Announcement on Live Port and User Port after every accepted switch |
| E | `-follow`: answer each User press with a toggle. | Works: pads switch cable at each press |

## The black screen (2026-10-03, macOS)

First Mac test of `pushapp -keep-user-mode -module monitor`. Steps 1 and 2
worked. When Live started, the Push screen and all LEDs went black and stayed
black after Live and `pushapp` were closed. Only a hard reset (hold power)
brought it back.

What the evidence says:

- `pushapp` did not hang. It drew 1,920 frames in 64 s (29.9 fps) with no
  display or write error, and it exited normally after Ctrl+C. The Push stopped
  showing the frames while the host kept sending them.
- Live announces Live Mode about 8 s after it launches, right after its Push
  helper finishes its handshake (14:34:26 start, 14:34:34.7 announcement; the
  Live log shows a script re-initialisation just before). The first keeper
  answered that announcement within 1 ms, in the middle of the handshake. It
  logged nothing, so we do not know how many switches it sent.
- Control run, with the keeper off: `pushapp` held the screen, the host put Push
  into User Mode once, then Live started. Live set Live Mode at +8 s, the pads
  followed Live, the screen stayed with `pushapp`, nothing went black. So a
  host-set User Mode before Live starts is harmless.
- A clean run of the keeper with no Live (screen idle, then a monitor module)
  also worked, including exit.

So the likely cause is the instant answer in the middle of Live's handshake.
Not proven. Changes made because of it: the keeper waits 3 s of quiet before
asking for User Mode (every new announcement moves the deadline), asks once per
announcement, and allows at most 3 switches in 30 s. It logs every
announcement and every request.

Result of the test of the new keeper with Live (macOS, 2026-10-03, 14:39):
no black screen, no wedge. Live launched at 14:39:16. Push announced Live Mode
at 14:39:23.97, the keeper asked for User Mode at 14:39:27.16 (once), Push
confirmed, and Ctrl+C gave Live Mode back and exited normally.

LED finding from the same test: the LEDs lit with Live's colors when Live set
Live Mode (expected: Live has the controls for those seconds). After the keeper
put User Mode back, they STAYED lit with Live's colors while the screen said
"User Mode Active". A mode switch does not clear LEDs. It only decides who may
change them from then on. Fix: each time Push confirms User Mode, PTA blanks
every pad and every LED button (`Port.ClearAllLEDs`, `pushmap.LEDButtons`, 74
buttons without the encoders), puts back what the active module lit (the host
records each module LED write), and lights the User button white. Not yet
tested on the device.

## Design

Decided with the user on 2026-10-03. The feature is called **Keep Push in User
Mode**.

- While it is on and PTA runs, Push stays in User Mode. When something sets
  Live Mode (Live does this when it starts), PTA sets User Mode again. While it
  is off, or after PTA exits, Live may set Live Mode.
- The checkbox is the only control. If the user presses User on the Push to
  leave User Mode, PTA puts it back. This is intended.
- The session uses the User Port. Auto-detect opens the Live Port, so with the
  option on, `bootstrap.Open` switches to the User Port of the same unit
  (`pickUserRef`). On Windows the role is empty, so the cable position decides:
  cable 2 is the User Port.
- PTA sends the switch on that User Port output. It answers each announcement
  of Live Mode with User Mode.
- PTA does not answer a Live Mode announcement at once. It waits 3 s with no new
  announcement, then asks for User Mode once. See "The black screen".
- Safety limit: at most 3 switches in 30 seconds. Over that, PTA stops, logs one
  warning and tries again after the window. This prevents a loop with another
  program.
- The User button LED is white while PTA keeps User Mode. Each time Push
  confirms User Mode, PTA blanks all LEDs, puts back the LEDs the active module
  lit, and lights the User button. Nothing else is lit by the host.
- With the option on, no module starts by itself. The whole screen says "User
  Mode Active" until a module is activated (it says "Entering User Mode" until
  Push has confirmed the mode). After a module is active, a strip with the
  same words shows for 3 seconds each time Push enters User Mode
  (`userModeBannerFor`).
- On exit, or when the option goes off, PTA gives Live Mode back and puts out the
  User button LED. This leaves Push ready for Live.
- Entering User Mode at connect is part of turning the option on, so the start
  order is: PTA, then Live.

### Phases

1. **Done and tested on macOS:** `internal/midi` (announcement event, SysEx listening,
   `Port.SendModeSwitch`), `internal/host/usermode.go` (policy, rate limit),
   `Runtime.SetKeepUserMode`, the banner, `bootstrap.Options.KeepUserMode`,
   `pushapp -keep-user-mode`. Unit tests with no hardware.
2. Mac test with `pushapp -keep-user-mode`. Results here.
3. **Built, UI not yet tried by hand:** `pushapp-ui`. A check box in the
   pairing form (`ConnectRequest.KeepUserMode`), and a check box on each session
   card that holds the User Port (`PushService.SetKeepUserMode`, refused on a
   Live Port session). With the option on at connect, no module is activated, so
   the idle screen shows. The session records the cable it really holds. Not
   remembered between runs (the user chooses at each pairing). MANUAL.md has the
   new start order, `docs/protocol/midi-input.md` the protocol facts,
   `docs/guides/debugging.md` the tool.
4. Windows test: the cable names are `Ableton Push 3 MIDI` (Live), `MIDIOUT2`
   (User) and `MIDIOUT3` (External). WinMM may not allow PTA and Live to open
   the same Push cable. The User Port must work for both.

## Done when

- The results are in `docs/protocol/midi-input.md` (done on the same date).
- The decisions above are made, and the option is built or dropped.
