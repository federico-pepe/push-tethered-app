package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"sync"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/bootstrap"
	"github.com/federico-pepe/push-tethered-app/internal/host"
	"github.com/federico-pepe/push-tethered-app/internal/identify"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
	"github.com/federico-pepe/push-tethered-app/internal/midiout"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// identifyDuration and identifyColour are shared by every identify call this
// process makes before a unit is connected — see hostManager.identifyUnit and
// identifyMIDIPort. Kept short: this is "which box is this", not a light show.
const identifyDuration = 3 * time.Second

// identifySwatch is orange, matching cmd/identifytest's probe colour.
var identifySwatch = color.NRGBA{R: 255, G: 140, B: 0, A: 255}

// session is one connected Push, from claim to teardown. hostManager can hold
// several at once — one per physical unit.
type session struct {
	key string // opaque, assigned by hostManager.connect; immune to port renumbering
	// unit is the stable identity used for lastErrs and the pairing UI: the
	// display selector when one was given, else the MIDI unit key. An error
	// must stay attributable to a physical box after its session is gone,
	// which a session key (freed on disconnect) cannot do.
	unit string

	rt      *host.Runtime
	cleanup func()
	cancel  context.CancelFunc
	stopped chan struct{} // closed by watch once Run has returned and teardown is done

	deliberate bool // set by disconnect/shutdownAll before cancelling, so watch knows this exit was asked for

	displaySel string
	midiIn     pmidi.PortRef
}

// ConnectRequest names the unit and MIDI cable to pair, both optional: an
// empty ConnectRequest reproduces the single-device auto-detect this app has
// always done (see bootstrap.Open), which still works as long as only one
// Push is attached.
type ConnectRequest struct {
	MIDIIn     pmidi.PortRef `json:"midiIn"`
	DisplaySel string        `json:"displaySel"`
	ModuleID   string        `json:"moduleId"` // "" activates the first available module
}

// unitKey derives the stable identity for a request — the display selector
// when the caller gave one, else whatever the MIDI pairing already resolved
// as this cable's unit, else the raw port name as a last resort so an
// auto-detected single-unit connect still has something to key lastErrs by.
func (r ConnectRequest) unitKey() string {
	return unitKeyFor(r.DisplaySel, r.MIDIIn)
}

// unitKeyFor is unitKey's logic, factored out so it can be applied a second
// time to the *resolved* identity after an auto-detect connect — see
// connect's use of rt.DisplayInfo/rt.MIDIRef, which is what makes an
// auto-detected session's unit key (and displaySel/midiIn) the unit that was
// actually claimed rather than the empty selector that was requested.
func unitKeyFor(displaySel string, midiIn pmidi.PortRef) string {
	switch {
	case displaySel != "":
		return displaySel
	case midiIn.Unit != "":
		return midiIn.Unit
	case midiIn.InName != "":
		return midiIn.InName
	default:
		return "auto"
	}
}

// hostManager owns every connected *host.Runtime across its whole
// disconnected/connected lifecycle, one per physical unit. It exists because
// bootstrap.Open can fail at the MIDI step — most commonly on Windows, where
// the Live port can't be found by name (see internal/midi's OpenNamed doc),
// or because more than one Push is attached and nothing can auto-pick — and
// the window must still open so the user has somewhere to pair from.
type hostManager struct {
	rootCtx  context.Context // cancelled on app quit/SIGINT/SIGTERM; parent of every session's context
	baseOpts bootstrap.Options

	// newModules builds a fresh set of module instances for one session.
	// Sessions must never share module instances: a module holds its own
	// running state on the struct itself (seq's current step and ticker,
	// for instance), so two Runtimes activating the same *seq.Module would
	// fight over one shared timer instead of each running its own —
	// confirmed live 2026-08-19, where starting seq on a second unit froze
	// the first unit's pad grid mid-sequence.
	newModules func() []module.Module

	mu       sync.Mutex
	sessions map[string]*session
	order    []string // insertion order, for a stable session listing
	nextKey  int

	// lastErrs is keyed by unit, not by session key, so a disconnect reason
	// survives the session that produced it and can still be shown on that
	// unit's row in the pairing view.
	lastErrs map[string]error

	// open replaces bootstrap.Open in tests — see hostmanager_test.go. Real
	// runs never override it.
	open func(bootstrap.Options) (*host.Runtime, func(), error)
}

func newHostManager(rootCtx context.Context, opts bootstrap.Options, newModules func() []module.Module) *hostManager {
	return &hostManager{
		rootCtx:    rootCtx,
		baseOpts:   opts,
		newModules: newModules,
		sessions:   map[string]*session{},
		lastErrs:   map[string]error{},
		open:       bootstrap.Open,
	}
}

// sessionInfo is hostManager.list's return shape — a snapshot, not a live
// handle, so callers can range over it without holding any lock.
type sessionInfo struct {
	Key        string
	Unit       string
	DisplaySel string
	MIDIIn     pmidi.PortRef
}

// list returns every connected session, in the order they were connected.
func (m *hostManager) list() []sessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sessionInfo, 0, len(m.order))
	for _, k := range m.order {
		s := m.sessions[k]
		out = append(out, sessionInfo{Key: s.key, Unit: s.unit, DisplaySel: s.displaySel, MIDIIn: s.midiIn})
	}
	return out
}

// session looks up a connected session's Runtime by key.
func (m *hostManager) session(key string) (*host.Runtime, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[key]
	if !ok {
		return nil, false
	}
	return s.rt, true
}

// ports lists every MIDI input port name the OS sees, for the raw fallback
// list — see PushService.ListMIDIPorts.
func (m *hostManager) ports() []string {
	return pmidi.ListInPorts()
}

// lastErrors returns the most recent unexpected disconnect reason for every
// unit that has one, keyed the same way sessionInfo.Unit and ConnectRequest's
// resolved unit key are.
func (m *hostManager) lastErrors() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.lastErrs))
	for k, err := range m.lastErrs {
		out[k] = err.Error()
	}
	return out
}

// conflict reports why req cannot be connected because it names a resource
// another live session already holds, or "" if there is none. A pure check
// over already-recorded session fields — no I/O, so it runs before the
// (slow, hardware-touching) open call and is what makes "already connected"
// a resource-dedup question rather than the single global gate this used to
// be.
func (m *hostManager) conflict(req ConnectRequest) string {
	for _, s := range m.sessions {
		if req.DisplaySel != "" && s.displaySel == req.DisplaySel {
			return fmt.Sprintf("that screen (%s) is already in use by session %s", req.DisplaySel, s.key)
		}
		if req.MIDIIn.InName != "" && s.midiIn.InName != "" && s.midiIn.InNum == req.MIDIIn.InNum {
			return fmt.Sprintf("that MIDI port (%s) is already in use by session %s", req.MIDIIn.InName, s.key)
		}
	}
	return ""
}

// connect claims the hardware named by req and starts a new session, returning
// its key. Refuses only when req names a unit or cable a live session already
// holds — connecting a second, different unit while the first is running is
// the normal case, not an error.
func (m *hostManager) connect(req ConnectRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if reason := m.conflict(req); reason != "" {
		return "", fmt.Errorf("%s", reason)
	}

	// Assigned before opening, not after, because it also names this
	// session's MIDI-out port (below) — two sessions both defaulting to
	// midiout.DefaultName would otherwise create two identically-named
	// virtual ports, and a DAW subscribing to that name would get an
	// arbitrary one of the two. Observed live in testing: both sessions'
	// logs read `MIDI out: "Push Tethered App" (virtual)` before this fix.
	m.nextKey++
	n := m.nextKey
	key := fmt.Sprintf("s%d", n)

	opts := m.baseOpts
	opts.MIDIIn = req.MIDIIn
	opts.DisplaySel = req.DisplaySel
	opts.Modules = m.newModules()
	if opts.MIDIOutName == "" && n > 1 {
		opts.MIDIOutName = fmt.Sprintf("%s %d", midiout.DefaultName, n)
	}

	rt, cleanup, err := m.open(opts)
	if err != nil {
		return "", err
	}

	moduleID := req.ModuleID
	if moduleID == "" {
		mods := rt.List()
		if len(mods) == 0 {
			cleanup()
			return "", fmt.Errorf("host: no modules available")
		}
		moduleID = mods[0].ID
	}
	if err := rt.Activate(moduleID); err != nil {
		cleanup()
		return "", fmt.Errorf("host: %w", err)
	}

	// Resolve what was actually claimed, not merely what was requested: an
	// auto-detect connect (req.DisplaySel == "" and req.MIDIIn a zero
	// PortRef) still claims one specific physical unit, and the session's
	// identity has to reflect that — otherwise the pairing view keeps
	// showing an auto-connected unit's screen as "unpaired" forever, since
	// its displaySel never matched anything real. Confirmed live 2026-08-19.
	displaySel := req.DisplaySel
	if displaySel == "" {
		if info, ok := rt.DisplayInfo(); ok {
			displaySel = info.ID
		}
	}
	midiIn := req.MIDIIn
	if midiIn.InName == "" {
		midiIn = rt.MIDIRef()
	}
	unit := unitKeyFor(displaySel, midiIn)

	ctx, cancel := context.WithCancel(m.rootCtx)
	runDone := make(chan error, 1)
	go func() { runDone <- rt.Run(ctx) }()

	sess := &session{
		key: key, unit: unit, rt: rt, cleanup: cleanup, cancel: cancel,
		stopped: make(chan struct{}), displaySel: displaySel, midiIn: midiIn,
	}
	m.sessions[key] = sess
	m.order = append(m.order, key)
	delete(m.lastErrs, unit)

	// watch is the sole reader of runDone. It fires whether Run stopped on its
	// own (e.g. the device was unplugged) or because disconnect/shutdownAll
	// cancelled its context; either way it tears the session down and removes
	// it from m.sessions so list() reflects reality instead of a dead entry.
	go m.watch(sess, runDone)
	return key, nil
}

func (m *hostManager) watch(sess *session, runDone chan error) {
	err := <-runDone
	defer close(sess.stopped)

	m.mu.Lock()
	if m.sessions[sess.key] != sess {
		m.mu.Unlock()
		return // this session was already torn down by something else
	}
	m.mu.Unlock()

	sess.rt.Shutdown()
	sess.cleanup()

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sess.key)
	for i, k := range m.order {
		if k == sess.key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	if err != nil && !sess.deliberate {
		m.lastErrs[sess.unit] = err
		log.Printf("host: session %s (%s) disconnected: %v", sess.key, sess.unit, err)
	}
}

// disconnect stops one session and releases its hardware. Waits for teardown
// to finish (including the LED clear in Shutdown) before returning.
func (m *hostManager) disconnect(key string) error {
	m.mu.Lock()
	sess, ok := m.sessions[key]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("no such session %q", key)
	}
	sess.deliberate = true
	cancel, stopped := sess.cancel, sess.stopped
	m.mu.Unlock()

	cancel()
	<-stopped
	return nil
}

// shutdownAll stops every session in parallel, each bounded so one wedged
// session cannot hold the process open indefinitely on quit. This is the one
// place a bug here could violate "always clear LEDs on every exit path": every
// session's Shutdown (which clears its LEDs) runs inside watch, not inside
// this function, so shutdownAll only signals and waits — it must wait for
// all of them, not just cancel and return.
func (m *hostManager) shutdownAll() {
	m.mu.Lock()
	sessions := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		s.deliberate = true
		sessions = append(sessions, s)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range sessions {
		wg.Add(1)
		go func(s *session) {
			defer wg.Done()
			s.cancel()
			select {
			case <-s.stopped:
			case <-time.After(5 * time.Second):
				log.Printf("host: session %s did not shut down within 5s", s.key)
			}
		}(s)
	}
	wg.Wait()
	log.Print("host: all sessions shut down")
}

// identifyUnit flashes sel's display for identifyDuration. Only meaningful for
// a unit no session has claimed yet — the pairing view only offers this for
// units not already shown as a session card. Claiming an already-connected
// unit's display would return display.ErrAlreadyClaimed, which is itself a
// clear enough error rather than a silent no-op.
func (m *hostManager) identifyUnit(sel string) error {
	return identify.Flash(m.rootCtx, sel, sel, identifySwatch, identifyDuration, 12)
}

// identifyMIDIPort flashes every pad on outNum for identifyDuration — see
// internal/identify.FlashLEDs for why this takes a bare port number rather
// than a PortRef (it is the disambiguation path for exactly the cables a
// PortRef could not resolve).
func (m *hostManager) identifyMIDIPort(outNum int) error {
	// Palette index 21 — a distinct, easy-to-spot colour confirmed on real
	// Push 3 hardware (core/push3/colors.go).
	return identify.FlashLEDs(m.rootCtx, outNum, 21, identifyDuration)
}
