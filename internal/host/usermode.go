package host

import (
	"sync"
	"time"

	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
)

// Keep User Mode: while it is on, the host puts Push into User Mode and puts it
// back whenever something else sets Live Mode (Live does this when it starts).
// While it is off, or after Shutdown, Live is free to set Live Mode.
//
// Why a loop is needed: the Push does not switch by itself. It answers each
// mode switch that a host sends with an announcement (the same bytes) on every
// cable. So the keeper sees every change, its own and Live's.
// Measured 2026-10-03: docs/protocol/midi-input.md, "Switching User Mode from
// the host". Plan: plans/2026-10-03-user-mode-switch.md.

const (
	// When something sets Live Mode, the keeper does not answer at once. It
	// waits keepSettle with no further announcement, then asks for User Mode
	// one time. Reason: Live sets Live Mode when its Push helper finishes its
	// start-up handshake (about 8 s after Live launches, measured 2026-10-03).
	// An answer 1 ms later landed in the middle of that handshake, and the
	// Push stopped showing the screen and had to be reset by hand
	// (plans/2026-10-03-user-mode-switch.md, "The black screen").
	keepSettle = 3 * time.Second

	// A fight with another program must not become a MIDI storm. At most
	// keepMaxSends mode switches in keepWindow. Over that, the keeper stops,
	// logs once, and tries again when the window has passed.
	keepMaxSends = 3
	keepWindow   = 30 * time.Second

	// userModeBannerFor is how long "User Mode Active" shows on the screen
	// after Push enters User Mode.
	userModeBannerFor = 3 * time.Second
)

// modeKeeper holds the policy. It does no I/O of its own: Runtime gives it the
// functions that write to the port and the screen. That keeps it testable
// without hardware.
type modeKeeper struct {
	send    func(pmidi.Mode) error // one mode switch to Push
	lampOff func()                 // the User button LED goes out
	entered func()                 // Push confirmed User Mode: refresh LEDs, banner
	logf    func(format string, args ...any)
	now     func() time.Time

	mu     sync.Mutex
	on     bool
	last   pmidi.Mode // last mode Push announced. Live until told otherwise.
	sends  []time.Time
	due    time.Time // when to ask for User Mode again. Zero: nothing pending.
	warned bool
}

func newModeKeeper(send func(pmidi.Mode) error, lampOff, entered func(), logf func(string, ...any)) *modeKeeper {
	return &modeKeeper{send: send, lampOff: lampOff, entered: entered, logf: logf, now: time.Now}
}

// SetOn turns keeping on or off. On: ask for User Mode now. Off: give Live
// Mode back and put out the User button LED.
func (k *modeKeeper) SetOn(on bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.on == on {
		return
	}
	k.on = on
	k.due = time.Time{}
	if on {
		k.warned = false
		k.enforce() // now: nothing is mid-handshake when the user turns this on
		return
	}
	k.lampOff()
	k.switchTo(pmidi.LiveMode)
}

// InUserMode reports whether keeping is on and Push has confirmed User Mode.
func (k *modeKeeper) InUserMode() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.on && k.last == pmidi.UserMode
}

// On reports whether keeping is on.
func (k *modeKeeper) On() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.on
}

// Seen takes a mode announcement from Push.
func (k *modeKeeper) Seen(m pmidi.Mode) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.logf("keep User Mode: Push announced %s Mode (keeping %v)", m, k.on)
	k.last = m
	if m == pmidi.UserMode {
		k.due = time.Time{} // we are where we want to be
		if k.on {
			k.entered()
		}
		return
	}
	if k.on {
		// Someone set Live Mode. Wait for quiet, then put User Mode back.
		// Every new announcement moves the deadline.
		k.due = k.now().Add(keepSettle)
		k.logf("keep User Mode: will ask for User Mode in %s if Push stays quiet", keepSettle)
	}
}

// Tick acts on a due request. Call it a few times a second.
func (k *modeKeeper) Tick() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.on || k.last != pmidi.LiveMode || k.due.IsZero() || k.now().Before(k.due) {
		return
	}
	k.due = time.Time{}
	k.enforce()
}

// Release gives Live Mode back for good. Called from Shutdown. No-op when
// keeping is off, so a session that never kept User Mode never touches it.
// Reports whether it sent anything.
func (k *modeKeeper) Release() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.on {
		return false
	}
	k.on = false
	k.due = time.Time{}
	k.lampOff()
	k.switchTo(pmidi.LiveMode)
	return true
}

// enforce asks for User Mode, within the rate limit. Caller holds k.mu.
func (k *modeKeeper) enforce() {
	now := k.now()
	recent := k.sends[:0]
	for _, t := range k.sends {
		if now.Sub(t) < keepWindow {
			recent = append(recent, t)
		}
	}
	k.sends = recent
	if len(k.sends) >= keepMaxSends {
		k.due = k.sends[0].Add(keepWindow) // try again when the window has passed
		if !k.warned {
			k.warned = true
			k.logf("keep User Mode: Push was set to Live Mode %d times in %s. Pausing. Is another program fighting for the mode?",
				keepMaxSends, keepWindow)
		}
		return
	}
	k.sends = append(k.sends, now)
	k.switchTo(pmidi.UserMode)
}

func (k *modeKeeper) switchTo(m pmidi.Mode) {
	k.logf("keep User Mode: asking Push for %s Mode", m)
	if err := k.send(m); err != nil {
		k.logf("keep User Mode: sending %s mode: %v", m, err)
	}
}
