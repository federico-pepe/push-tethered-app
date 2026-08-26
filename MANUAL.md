# Push Tethered App — Manual

This is the end-user manual: how to run the app, pair a Push, and configure
it, including how to run it alongside Ableton Live. For information about
the project itself, see [README.md](README.md). For protocol and hardware
reference for contributors, see [docs/](docs/).

## Pairing a Push

Open `pushapp-ui`. Before you connect a Push, the main window shows two
lists: unpaired screens and unpaired MIDI ports.

1. Pick one item from each list.
2. Click **Pair and connect**.

If you have more than one Push and cannot tell them apart, click
**Identify**. Identify blinks the screen, or lights the pads of the MIDI
port, that you picked.

After you connect one Push, the pairing controls move to a **Settings…**
button. Click **Settings…** to pair another unit or to change the MIDI port
of a connected unit.

Each connected session has a card with a small triangle on the left. Click
the triangle to fold away the module list of that card. Use this when you
connect several units and want to see only one at a time.

A MIDI port's name shows which physical cable it is. Every Push has up to
three:

| Port | What it is for |
|---|---|
| **Live Port** | The cable that Ableton Live uses. Pick this port for a normal session without Live. |
| **User Port** | Active only while **User Mode** is on at the device (press the User button). Pick this port to run alongside Live — see below. |
| **External Port** | Push 3's MIDI DIN connector on the back. Not related to Live. |

## Running alongside Ableton Live

By default, Live and this app cannot use Push at the same time. A
background helper in Live claims the screen. Without User Mode, Live and
this app also compete for control of the pads and their LEDs.

User Mode on Push solves this problem. While User Mode is on, Live cannot
read pad presses or set LED colors. This app controls the pads instead, and
Live runs normally otherwise.

**Do these steps in this exact order:**

1. If Live is running, quit Live.
2. Open `pushapp-ui`. Pick the screen and the **User Port**. Click **Pair
   and connect**.

   Note: Do this step while Live is closed. The app must claim the screen
   before Live starts. If Live starts first, Live keeps the screen, and you
   cannot take it back afterward.
3. Launch Live.
4. Press **User** on the Push hardware. This turns on User Mode.

Your module now keeps the screen. It receives pad presses and can set pad
colors, while Live runs normally.

Live's own control-surface features, such as Session View coloring and pad
input, still work inside Live. But User Mode blocks them from reaching the
physical pads of Push.

To return to normal Live control, press **User** again. This turns off User
Mode.

### Recording a module's MIDI into Live

Some modules send MIDI output as well as pad input — the step sequencer is
one example. Other software, including Live, sees this output as a virtual
MIDI port.

To record this output in Live:

1. Add a MIDI track in Live.
2. Set the track's input to the app's output port.
3. Arm the track to record.

### Syncing a module to an external clock

The app can also receive MIDI from other software or hardware. This input
is separate from the MIDI output above and from the controls of Push.

A module that supports this feature can follow an incoming MIDI clock
instead of its own tempo. Not every module supports this feature.

To connect a clock source, point the MIDI output of the sending device or
software at the MIDI input port of this app. This is the same type of
connection you use to point a synthesizer at Live, but in the opposite
direction.

The step sequencer module locks to an incoming clock automatically. Start
and stop the sequencer from the sending device; the app has no setting for
this. If the clock stops, the sequencer returns to its own tempo within a
few seconds.

To check the clock connection before you use it in a module you care about,
run the **Beat Counter** module. This module lights one pad and shows a
number from 1 to 4 for each beat. It is the simplest way to check that a
clock signal arrives.

## Live screen mirror

Both `pushapp` and `pushapp-ui` can stream a live copy of the Push screen to
a browser. Use this feature to demo the app without the physical device in
view, or to debug the drawing of a module without looking at the hardware.

**In `pushapp-ui`:** each connected session's card has a **Live screen**
button.

1. Click **Live screen** to open a live view in the app window.
2. Click **Hide live screen**, or the × on the overlay, to close the view.
3. Click **Open in browser** to open the same stream in your default
   browser. Use this option to share only the Push display during a demo.

Each connected Push has its own stream, at
`http://localhost:3000/screen/<session key>`. This lets you see the correct
stream when more than one unit is connected.

**In `pushapp` (CLI):** the stream is on by default at
`http://localhost:3000/screen`. Run the app, then open that URL in a
browser. To use a different address, pass `-mirror-addr`. To turn off the
stream, pass `-mirror-addr=""`:

```bash
go run ./cmd/pushapp -mirror-addr localhost:8080  # different port
go run ./cmd/pushapp -mirror-addr=""              # disabled
```

The stream uses no extra resources when no one watches it. Encoding starts
only when a browser tab opens the stream, and stops immediately when the
tab closes.

## Troubleshooting

**`display interface is claimed by another process (Live?)`** — Another
process, usually Live, already owns the screen of Push.

If you want this app to control the screen, quit Live, pair the app, then
start Live again. See the order of steps above.

This app does not retry the screen claim on its own. After you fix the
conflict, reconnect the session instead of waiting.

**A MIDI port row is gray and shows the message `Matches another identical
unit`** — You have two Push units of the same model. Their MIDI ports
report identical names, so the app cannot tell them apart on its own.

Click **Identify** on each candidate port. Watch which pads light up to
find the correct unit.

**The screen and the MIDI unit of a MIDI port row do not match** — A red
warning appears when the screen and the MIDI port you picked report
different Push models. This usually means you picked the wrong pair.

Use **Identify** to check which screen matches which MIDI unit before you
pair them.

**Nothing lights up, and nothing is heard, on the User Port** — Make sure
that User Mode is on at the device. Press **User** on the hardware;
selecting the port in the app is not enough.

LED output and pad input on the User Port work only while User Mode is on.
