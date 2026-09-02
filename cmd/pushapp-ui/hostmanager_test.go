package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/bootstrap"
	"github.com/federico-pepe/push-tethered-app/internal/host"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// fakeModule is the minimum module.Module a Runtime needs to construct and
// activate without ever touching hardware — Draw/Handle are never exercised
// here since these tests stop short of calling rt.Run.
type fakeModule struct{}

func (fakeModule) Meta() module.Meta        { return module.Meta{ID: "fake", Name: "Fake"} }
func (fakeModule) Init(h module.Host) error { return nil }
func (fakeModule) Handle(e module.Event)    {}
func (fakeModule) Draw(f *module.Frame)     {}
func (fakeModule) Close() error             { return nil }

// fakeModules is the newModules factory these tests pass to newHostManager.
// Each fake open below builds its own Runtime directly and ignores
// opts.Modules, so this exists only to satisfy newHostManager's signature —
// module-instance freshness itself is exercised by
// TestConnectGivesEachSessionFreshModuleInstances below, which uses the real
// factory shape.
func fakeModules() []module.Module { return []module.Module{fakeModule{}} }

// newTestManager builds a hostManager whose open seam constructs a real
// host.Runtime with no hardware at all (nil port, nil display). host.Run
// degrades gracefully with both nil — same as a real ErrBusy/MIDI-only
// session degrades with a nil display — so the spawned Run goroutine stays
// alive respecting context cancellation instead of erroring out immediately,
// which is what makes a session's lifecycle (stays in list() until
// disconnected) testable here without hardware.
func newTestManager(t *testing.T) *hostManager {
	t.Helper()
	m := newHostManager(context.Background(), bootstrap.Options{FPS: 30}, fakeModules)
	m.open = func(opts bootstrap.Options) (*host.Runtime, func(), error) {
		rt, err := host.New(nil, nil, host.Options{FPS: opts.FPS, NoDisplay: true}, fakeModule{})
		if err != nil {
			return nil, nil, err
		}
		return rt, func() {}, nil
	}
	return m
}

func TestConnectAssignsDistinctSessionKeys(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()

	key1, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"})
	if err != nil {
		t.Fatalf("connect 1: %v", err)
	}
	key2, err := m.connect(ConnectRequest{DisplaySel: "usb:1.2"})
	if err != nil {
		t.Fatalf("connect 2: %v", err)
	}
	if key1 == key2 {
		t.Errorf("expected distinct session keys, got %q twice", key1)
	}
}

func TestConnectRefusesSameDisplayTwice(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	_, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"})
	if err == nil {
		t.Fatal("second connect to the same display: expected a conflict error, got nil")
	}
}

func TestConnectAllowsDifferentDisplaysConcurrently(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("connect to usb:1.1: %v", err)
	}
	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.2"}); err != nil {
		t.Fatalf("connect to usb:1.2 while usb:1.1 is live: %v", err)
	}
	if got := len(m.list()); got != 2 {
		t.Errorf("list() = %d sessions, want 2", got)
	}
}

func TestConnectRefusesSameMIDICableTwice(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()

	ref := pmidi.PortRef{InName: "Ableton Push 3 Live Port", InNum: 0, OutNum: 0}
	if _, err := m.connect(ConnectRequest{MIDIIn: ref, DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	// Different display selector, same MIDI cable — the MIDI conflict must
	// still fire; a unit is identified by either resource being already in
	// use, not just the display.
	_, err := m.connect(ConnectRequest{MIDIIn: ref, DisplaySel: "usb:1.2"})
	if err == nil {
		t.Fatal("expected a conflict error reusing the same MIDI cable, got nil")
	}
}

func TestUnitKeyPrefersDisplaySelector(t *testing.T) {
	req := ConnectRequest{DisplaySel: "usb:1.1", MIDIIn: pmidi.PortRef{Unit: "Ableton Push 3"}}
	if got := req.unitKey(); got != "usb:1.1" {
		t.Errorf("unitKey() = %q, want the display selector", got)
	}
}

func TestUnitKeyFallsBackToMIDIUnit(t *testing.T) {
	req := ConnectRequest{MIDIIn: pmidi.PortRef{Unit: "Ableton Push 3"}}
	if got := req.unitKey(); got != "Ableton Push 3" {
		t.Errorf("unitKey() = %q, want the MIDI unit", got)
	}
}

func TestUnitKeyFallsBackToInName(t *testing.T) {
	req := ConnectRequest{MIDIIn: pmidi.PortRef{InName: "Ableton Push 3 Live Port"}}
	if got := req.unitKey(); got != "Ableton Push 3 Live Port" {
		t.Errorf("unitKey() = %q, want the input port name", got)
	}
}

func TestUnitKeyDefaultsToAutoWhenEverythingIsEmpty(t *testing.T) {
	if got := (ConnectRequest{}).unitKey(); got != "auto" {
		t.Errorf("unitKey() = %q, want \"auto\"", got)
	}
}

// unitKeyFor is unitKey's logic, applied a second time in connect() to the
// *resolved* identity (rt.DisplayInfo/rt.MIDIRef) after an auto-detect
// connect — this is what fixes a real bug: an auto-connected session used to
// keep displaySel/midiIn empty forever (echoing the empty request instead of
// what was actually claimed), so the pairing view never recognized it as
// paired even though it was genuinely connected. This pins the standalone
// function's behaviour directly; the end-to-end resolution in connect()
// cannot be unit-tested the same way, since DisplayInfo/MIDIRef read real
// hardware-typed fields (r.dev/r.port) that a test-built Runtime leaves nil —
// verified live instead (plans/2026-08-19-multi-device.md).
func TestUnitKeyForPrefersDisplaySelector(t *testing.T) {
	if got := unitKeyFor("usb:1.1", pmidi.PortRef{Unit: "Ableton Push 3"}); got != "usb:1.1" {
		t.Errorf("unitKeyFor() = %q, want the display selector", got)
	}
}

func TestUnitKeyForResolvedFromRuntimeIdentity(t *testing.T) {
	// Simulates what connect() does for an auto-detect request: req was all
	// empty, but rt.DisplayInfo() resolved a real serial.
	resolvedDisplaySel := "serial:37589789"
	if got := unitKeyFor(resolvedDisplaySel, pmidi.PortRef{}); got != resolvedDisplaySel {
		t.Errorf("unitKeyFor() = %q, want the resolved display selector", got)
	}
}

// lastErrs is keyed by unit so a disconnect reason survives the session that
// produced it — this exercises that bookkeeping directly, without needing a
// real Run() failure to trigger it.
func TestDisconnectClearsSessionButNotConflict(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()

	key, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := m.disconnect(key); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if got := len(m.list()); got != 0 {
		t.Errorf("list() after disconnect = %d sessions, want 0", got)
	}
	// The unit is free again — a fresh connect to it must succeed, not be
	// refused as if the torn-down session were still live.
	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Errorf("reconnect after disconnect: %v", err)
	}
}

func TestDisconnectUnknownKeyErrors(t *testing.T) {
	m := newTestManager(t)
	if err := m.disconnect("no-such-session"); err == nil {
		t.Error("disconnect of an unknown key: expected an error, got nil")
	}
}

func TestSessionLookupMissKeyReturnsFalse(t *testing.T) {
	m := newTestManager(t)
	if _, ok := m.session("missing"); ok {
		t.Error("session(\"missing\") = ok, want !ok")
	}
}

func TestShutdownAllWaitsForEverySession(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("connect 1: %v", err)
	}
	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.2"}); err != nil {
		t.Fatalf("connect 2: %v", err)
	}

	done := make(chan struct{})
	go func() {
		m.shutdownAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdownAll did not return")
	}
	if got := len(m.list()); got != 0 {
		t.Errorf("list() after shutdownAll = %d sessions, want 0", got)
	}
}

// countingModule is a stateful module.Module used to prove instance
// freshness by pointer identity, not just by ID string — two sessions
// activating "the same seq module" by ID is fine; two sessions holding a
// pointer to the literal same *seq.Module is the bug that froze one unit's
// pad grid when seq was started on a second unit (2026-08-19).
type countingModule struct {
	// id is not read by anything — its only purpose is to give the struct a
	// nonzero size. A zero-size struct's allocations all alias the runtime's
	// shared zerobase address, which would make every *countingModule
	// compare equal regardless of whether connect() actually built a fresh
	// one — this field is what makes the pointer-identity check below mean
	// anything.
	id int
}

func (*countingModule) Meta() module.Meta      { return module.Meta{ID: "counting", Name: "Counting"} }
func (*countingModule) Init(module.Host) error { return nil }
func (*countingModule) Handle(module.Event)    {}
func (*countingModule) Draw(*module.Frame)     {}
func (*countingModule) Close() error           { return nil }

// This is the real bug, reproduced directly: main.go used to call
// availableModules() once and hand the same slice of module instances to
// every session via bootstrap.Options.Modules, set once at hostManager
// construction. Two sessions activating the same *seq.Module fought over one
// shared ticker and step counter instead of each running its own. The fix is
// hostManager.newModules, called fresh inside connect — this test calls the
// real connect path (not a fake open that ignores opts.Modules, like the
// other tests in this file) so it actually exercises that call.
func TestConnectGivesEachSessionFreshModuleInstances(t *testing.T) {
	var built []*countingModule
	factory := func() []module.Module {
		m := &countingModule{id: len(built)}
		built = append(built, m)
		return []module.Module{m}
	}

	m := newHostManager(context.Background(), bootstrap.Options{FPS: 30}, factory)
	m.rootCtx, _ = uncancelledContext()
	m.open = func(opts bootstrap.Options) (*host.Runtime, func(), error) {
		rt, err := host.New(nil, nil, host.Options{FPS: opts.FPS, NoDisplay: true}, opts.Modules...)
		if err != nil {
			return nil, nil, err
		}
		return rt, func() {}, nil
	}

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("connect 1: %v", err)
	}
	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.2"}); err != nil {
		t.Fatalf("connect 2: %v", err)
	}

	if len(built) != 2 {
		t.Fatalf("newModules called %d times, want 2 (once per session)", len(built))
	}
	if built[0] == built[1] {
		t.Error("both sessions received the same module instance, want distinct instances per session")
	}
}

// A real pairing session hit this: two sessions both defaulting to
// midiout.DefaultName created two identically-named virtual MIDI-out ports,
// so a DAW subscribing by name would get an arbitrary one of the two.
func TestConnectAssignsDistinctMIDIOutNames(t *testing.T) {
	var gotNames []string
	m := newHostManager(context.Background(), bootstrap.Options{FPS: 30}, fakeModules)
	m.rootCtx, _ = uncancelledContext()
	m.open = func(opts bootstrap.Options) (*host.Runtime, func(), error) {
		gotNames = append(gotNames, opts.MIDIOutName)
		rt, err := host.New(nil, nil, host.Options{FPS: opts.FPS, NoDisplay: true}, fakeModule{})
		if err != nil {
			return nil, nil, err
		}
		return rt, func() {}, nil
	}

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("connect 1: %v", err)
	}
	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.2"}); err != nil {
		t.Fatalf("connect 2: %v", err)
	}

	if len(gotNames) != 2 {
		t.Fatalf("open called %d times, want 2", len(gotNames))
	}
	if gotNames[0] == gotNames[1] {
		t.Errorf("both sessions got the same MIDIOutName %q, want distinct names", gotNames[0])
	}
}

// An explicit MIDIOutName in baseOpts (e.g. from a future CLI flag) is the
// caller's choice and must not be overridden just because a second session
// exists — the distinct-naming fallback only kicks in for the empty default.
func TestConnectRespectsExplicitMIDIOutName(t *testing.T) {
	var gotNames []string
	m := newHostManager(context.Background(), bootstrap.Options{FPS: 30, MIDIOutName: "My Chosen Name"}, fakeModules)
	m.rootCtx, _ = uncancelledContext()
	m.open = func(opts bootstrap.Options) (*host.Runtime, func(), error) {
		gotNames = append(gotNames, opts.MIDIOutName)
		rt, err := host.New(nil, nil, host.Options{FPS: opts.FPS, NoDisplay: true}, fakeModule{})
		if err != nil {
			return nil, nil, err
		}
		return rt, func() {}, nil
	}

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("connect 1: %v", err)
	}
	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.2"}); err != nil {
		t.Fatalf("connect 2: %v", err)
	}
	for i, name := range gotNames {
		if name != "My Chosen Name" {
			t.Errorf("session %d got MIDIOutName %q, want the explicit choice preserved", i, name)
		}
	}
}

// CapturePath is process-wide (m.baseOpts), not per-request — unlike
// MIDIIn/DisplaySel there is no way for two sessions to choose the *same*
// path deliberately, because there is no per-session choice at all. A second
// session would collide on the literal only path available.
func TestConnectRefusesSecondSessionWhenCapturePathSet(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()
	m.baseOpts.CapturePath = "/tmp/capture.mp4"

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	_, err := m.connect(ConnectRequest{DisplaySel: "usb:1.2"})
	if err == nil {
		t.Fatal("second connect with CapturePath set and a session already live: expected an error, got nil")
	}
}

func TestConnectAllowsCapturePathWithNoLiveSession(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()
	m.baseOpts.CapturePath = "/tmp/capture.mp4"

	if _, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"}); err != nil {
		t.Fatalf("connect: %v", err)
	}
}

// shutdownAll must cancel every in-flight Identify call and wait for it to
// actually finish (blank its screen / clear its pads), not merely ask it to
// stop — otherwise quitting mid-flash leaves a unit lit past the point the
// app believes everything is shut down. This exercises beginIdentify and
// shutdownAll's cancel-and-wait directly, standing in for identify.Flash /
// identify.FlashLEDs (which need real hardware to run at all): the fake
// worker below blocks on ctx.Done(), exactly as both of those do internally.
func TestShutdownAllWaitsForInFlightIdentify(t *testing.T) {
	m := newTestManager(t)
	m.rootCtx, _ = uncancelledContext()

	ctx, done := m.beginIdentify()
	identifyFinished := make(chan struct{})
	go func() {
		<-ctx.Done() // stands in for identify.Flash/FlashLEDs' own ctx.Done() select
		close(identifyFinished)
		done()
	}()

	shutdownReturned := make(chan struct{})
	go func() {
		m.shutdownAll()
		close(shutdownReturned)
	}()

	select {
	case <-shutdownReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdownAll did not return")
	}

	select {
	case <-identifyFinished:
	default:
		t.Error("shutdownAll returned before the in-flight identify call's context was cancelled and it finished")
	}
}

func TestConnectPropagatesOpenError(t *testing.T) {
	m := newHostManager(context.Background(), bootstrap.Options{}, fakeModules)
	sentinel := errors.New("no Push found")
	m.open = func(bootstrap.Options) (*host.Runtime, func(), error) { return nil, nil, sentinel }

	if _, err := m.connect(ConnectRequest{}); !errors.Is(err, sentinel) {
		t.Errorf("connect error = %v, want %v", err, sentinel)
	}
}

// A failed connect must not advance nextKey. main.go calls connect once,
// unconditionally, for startup auto-detect, and that call routinely fails
// when no Push is plugged in yet or auto-detect can't resolve one (Windows,
// or more than one unit attached). Before this test's fix, that failure
// still incremented nextKey, so the user's first real, successful pairing
// became session 2 instead of 1 — and its default MIDI-out name became
// "Push Tethered App 2" (see TestConnectAssignsDistinctMIDIOutNames),
// which matches none of a Windows user's manually created loopback ports
// named after the un-suffixed default. Confirmed live 2026-09-02 against
// real Windows hardware: a fresh pushapp-ui launch with a single pairing
// attempt still produced session "s2".
func TestConnectDoesNotBurnSessionNumberOnFailedAttempt(t *testing.T) {
	var gotNames []string
	m := newHostManager(context.Background(), bootstrap.Options{FPS: 30}, fakeModules)
	m.rootCtx, _ = uncancelledContext()

	attempt := 0
	m.open = func(opts bootstrap.Options) (*host.Runtime, func(), error) {
		attempt++
		if attempt == 1 {
			return nil, nil, errors.New("no Push found")
		}
		gotNames = append(gotNames, opts.MIDIOutName)
		rt, err := host.New(nil, nil, host.Options{FPS: opts.FPS, NoDisplay: true}, fakeModule{})
		if err != nil {
			return nil, nil, err
		}
		return rt, func() {}, nil
	}

	if _, err := m.connect(ConnectRequest{}); err == nil {
		t.Fatal("connect 1: expected the simulated auto-detect failure, got nil error")
	}
	key, err := m.connect(ConnectRequest{DisplaySel: "usb:1.1"})
	if err != nil {
		t.Fatalf("connect 2: %v", err)
	}

	if key != "s1" {
		t.Errorf("session key = %q, want %q (a prior failed attempt must not consume a session number)", key, "s1")
	}
	if len(gotNames) != 1 {
		t.Fatalf("open called %d times with a name recorded, want 1", len(gotNames))
	}
	if gotNames[0] != "" {
		t.Errorf("MIDIOutName = %q, want empty (the un-suffixed default) for the first real session", gotNames[0])
	}
}

func uncancelledContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
