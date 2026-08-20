# Push Tethered App — Manual

This is the **end-user manual**: how to run the app, pair a Push, and
configure it correctly — including running it alongside Ableton Live. For
what the project *is* and why it exists, see [README.md](README.md). For
protocol/hardware reference and contributor docs, see [docs/](docs/).

## Pairing a Push

Open `pushapp-ui`. Unpaired screens and MIDI ports show up in two lists.
Pick one of each and click **Pair and connect**. Use **Identify** if you
have more than one Push and can't tell them apart by looking — it blinks
the picked screen or lights up the picked MIDI port's pads.

A MIDI port's name tells you which physical cable it is. Every Push exposes
up to three:

| Port | What it's for |
|---|---|
| **Live Port** | The cable Ableton Live uses. Pick this for a normal, Live-free session. |
| **User Port** | Active only while Push's own **User Mode** is engaged on the device (press the User button). Pick this to run alongside Live — see below. |
| **External Port** | Push 3's physical MIDI DIN connector on the back. Not related to Live. |

## Running alongside Ableton Live

Live and this app can't both use Push at the same time by default — Live's
own background helper claims the screen, and without any workaround it also
fights this app for the pads' MIDI and LEDs. Push's built-in **User Mode**
solves this: while it's engaged, Live is completely cut off from the pads
(both reading presses and painting LED colours), and this app gets them
instead — with Live still running normally otherwise.

**The order matters. Do it in exactly this sequence:**

1. **Quit Live** if it's running.
2. Open `pushapp-ui`, pick the screen and the **User Port**, and click
   **Pair and connect**. This has to happen while Live is closed — the
   screen claim only sticks if it happens first; Live launching afterward
   will not take it away, but Live launching *before* this step wins the
   screen instead, and there's no way to steal it back afterward.
3. **Now launch Live.**
4. **Press User on the Push hardware** to engage User Mode.

That's it — the module you're running keeps the screen, gets pad presses,
and can paint pad colours, all while Live runs. Live's own control-surface
features (Session View colouring, its pad input) are unaffected; they're
just not reaching Push's physical pads while User Mode is engaged.

To go back to normal Live control-surface use, press User again to exit
User Mode.

### Recording a module's MIDI into Live

Some modules (the step sequencer, for example) send MIDI out as well as
reading pad input. That output shows up as a virtual MIDI port other
software can see — including Live. Add a MIDI track in Live, set its input
to the app's output port, and arm it to record, same as any other MIDI
source.

### Syncing a module to an external clock

The app can also *receive* MIDI from other software or hardware, separate
from its own MIDI output above and from Push's own controls. A module built
to use it (not every module is) can be driven by an incoming MIDI clock, so
its tempo follows something else instead of running on its own. Point the
sending device or software's MIDI output at this app's input port — same
kind of connection as pointing a synth at Live, just in the other direction.

The step sequencer module locks to an incoming clock automatically when one
is connected — start/stop it from the sending side, no setting to flip. It
falls back to its own tempo within a couple of seconds of the clock
stopping. To just confirm the connection itself is working before trying it
on something you care about, run the **Beat Counter** module — it does
nothing but light one pad and show a number (1-4) per beat, the simplest
possible proof a clock is actually arriving.

## Troubleshooting

**"display interface is claimed by another process (Live?)"** — something
else (usually Live) already owns Push's screen. If you want this app to
have it, quit Live and pair first, then relaunch Live (see the ordering
above). This app never retries the claim automatically once it degrades —
if you fix the underlying conflict, reconnect the session rather than
waiting.

**A MIDI port row is greyed out and says "Matches another identical
unit"** — you have two Push units of the same model, and their MIDI ports
report identical names, so software alone can't tell them apart. Use
Identify on each candidate to find the right one by watching which pads
light up.

**A MIDI-port row's screen and MIDI unit look mismatched** — a red warning
appears if the screen you picked and the MIDI port you picked report
different Push models. That usually means you grabbed the wrong pair; use
Identify to confirm which screen goes with which MIDI unit before pairing.

**Nothing lights up / nothing is heard on User Port** — check that User
Mode is actually engaged on the device (press User, don't just have the
port selected in the app) — LED writes and pad input on User Port only work
while it's on.
