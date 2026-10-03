package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// rig: a keeper with a fake port, a fake clock, and a record of what it did.
type keeperRig struct {
	k        *modeKeeper
	sent     []pmidi.Mode
	lampOffs int
	entered  int
	logs     []string
	t        time.Time
	err      error
}

func newKeeperRig() *keeperRig {
	r := &keeperRig{t: time.Unix(1000, 0)}
	r.k = newModeKeeper(
		func(m pmidi.Mode) error { r.sent = append(r.sent, m); return r.err },
		func() { r.lampOffs++ },
		func() { r.entered++ },
		func(f string, a ...any) { r.logs = append(r.logs, f) },
	)
	r.k.now = func() time.Time { return r.t }
	return r
}

func (r *keeperRig) advance(d time.Duration) { r.t = r.t.Add(d) }

func (r *keeperRig) lastSent() pmidi.Mode { return r.sent[len(r.sent)-1] }

func TestTurningOnAsksForUserMode(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	if len(r.sent) != 1 || r.sent[0] != pmidi.UserMode {
		t.Fatalf("sent %v, want one User switch", r.sent)
	}
	r.k.SetOn(true) // already on: nothing more
	if len(r.sent) != 1 {
		t.Errorf("turning on twice sent again: %v", r.sent)
	}
}

func TestAnnouncementOfUserModeRefreshesTheLEDsAndBanner(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	r.k.Seen(pmidi.UserMode)
	if r.entered != 1 {
		t.Errorf("entered=%d, want one refresh", r.entered)
	}
	r.k.Seen(pmidi.UserMode) // back in User Mode after Live: refresh again
	if r.entered != 2 {
		t.Errorf("every confirmation must refresh, entered=%d", r.entered)
	}
}

func TestLiveModeAnnouncementIsNotAnsweredAtOnce(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	r.k.Seen(pmidi.UserMode)
	r.sent = nil
	r.advance(time.Second)
	r.k.Seen(pmidi.LiveMode) // Live's helper finished its handshake
	if len(r.sent) != 0 {
		t.Fatalf("answered within the handshake: %v", r.sent)
	}
	r.advance(keepSettle - time.Millisecond)
	r.k.Tick()
	if len(r.sent) != 0 {
		t.Fatalf("answered before Push had been quiet for %s: %v", keepSettle, r.sent)
	}
	r.advance(2 * time.Millisecond)
	r.k.Tick()
	if len(r.sent) != 1 || r.sent[0] != pmidi.UserMode {
		t.Errorf("after the quiet time want one User Mode request, got %v", r.sent)
	}
	r.k.Tick()
	if len(r.sent) != 1 {
		t.Errorf("one announcement must cause one request, got %v", r.sent)
	}
}

func TestMoreAnnouncementsMoveTheDeadline(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	r.k.Seen(pmidi.UserMode)
	r.sent = nil
	r.k.Seen(pmidi.LiveMode)
	r.advance(keepSettle - time.Second)
	r.k.Seen(pmidi.LiveMode) // still busy
	r.advance(keepSettle - time.Second)
	r.k.Tick()
	if len(r.sent) != 0 {
		t.Errorf("answered while Push was still busy: %v", r.sent)
	}
	r.advance(2 * time.Second)
	r.k.Tick()
	if len(r.sent) != 1 {
		t.Errorf("want one request after the second quiet time, got %v", r.sent)
	}
}

func TestUserModeAnnouncementCancelsAPendingRequest(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	r.k.Seen(pmidi.UserMode)
	r.sent = nil
	r.k.Seen(pmidi.LiveMode)
	r.advance(time.Second)
	r.k.Seen(pmidi.UserMode) // someone else already put it back
	r.advance(time.Minute)
	r.k.Tick()
	if len(r.sent) != 0 {
		t.Errorf("sent although Push is in User Mode: %v", r.sent)
	}
}

func TestNothingIsSentWhileKeepingIsOff(t *testing.T) {
	r := newKeeperRig()
	r.k.Seen(pmidi.LiveMode)
	r.k.Seen(pmidi.UserMode)
	r.k.Tick()
	r.k.Release()
	if len(r.sent) != 0 || r.lampOffs != 0 || r.entered != 0 {
		t.Errorf("an off keeper acted: sent=%v lampOffs=%d entered=%d", r.sent, r.lampOffs, r.entered)
	}
}

func TestTurningOffGivesLiveModeBackAndPutsOutTheLamp(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	r.k.Seen(pmidi.UserMode)
	r.k.SetOn(false)
	if r.lastSent() != pmidi.LiveMode || r.lampOffs != 1 {
		t.Errorf("sent=%v lampOffs=%d, want Live Mode and the lamp off", r.sent, r.lampOffs)
	}
	n := len(r.sent)
	r.k.Seen(pmidi.LiveMode) // the answer to our own switch: must not be fought
	if len(r.sent) != n {
		t.Errorf("an off keeper answered Live Mode: %v", r.sent)
	}
}

func TestReleaseGivesLiveModeBackOnlyWhenKeeping(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	r.k.Release()
	if r.lastSent() != pmidi.LiveMode || r.k.On() {
		t.Errorf("sent=%v on=%v", r.sent, r.k.On())
	}
	n := len(r.sent)
	r.k.Release()
	if len(r.sent) != n {
		t.Error("a second Release sent again")
	}
}

func TestFightWithAnotherProgramIsRateLimitedThenRetried(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true) // send 1, the first entry
	for i := 0; i < 8; i++ {
		r.k.Seen(pmidi.LiveMode)
		r.advance(keepSettle + time.Millisecond)
		r.k.Tick()
	}
	if len(r.sent) != keepMaxSends {
		t.Fatalf("sent %d switches in a fight, want the limit %d", len(r.sent), keepMaxSends)
	}
	if n := countContaining(r.logs, "Pausing"); n != 1 {
		t.Errorf("want exactly one warning, got %d: %v", n, r.logs)
	}
	r.advance(keepWindow)
	r.k.Tick()
	if len(r.sent) != keepMaxSends+1 || r.lastSent() != pmidi.UserMode {
		t.Errorf("want one retry after the window, sent %d", len(r.sent))
	}
}

func TestTickDoesNothingWhenPushIsAlreadyInUserMode(t *testing.T) {
	r := newKeeperRig()
	r.k.SetOn(true)
	r.k.Seen(pmidi.UserMode)
	r.advance(time.Minute)
	n := len(r.sent)
	r.k.Tick()
	if len(r.sent) != n {
		t.Errorf("Tick sent while in User Mode: %v", r.sent)
	}
}

func TestSendErrorIsLoggedNotFatal(t *testing.T) {
	r := newKeeperRig()
	r.err = errors.New("port closed")
	r.k.SetOn(true)
	if countContaining(r.logs, "sending") != 1 {
		t.Errorf("want the send error logged: %v", r.logs)
	}
}

func countContaining(lines []string, sub string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

// stubModule is the smallest module.Module, for a Runtime with no hardware.
type stubModule struct{}

func (stubModule) Meta() module.Meta      { return module.Meta{ID: "stub"} }
func (stubModule) Init(module.Host) error { return nil }
func (stubModule) Handle(module.Event)    {}
func (stubModule) Draw(*module.Frame)     {}
func (stubModule) Close() error           { return nil }

func TestBannerShowsUserModeActiveThenGoes(t *testing.T) {
	rt, err := New(nil, nil, Options{}, stubModule{})
	if err != nil {
		t.Fatal(err)
	}
	rt.drawUserModeBanner()
	if n := len(rt.frame.Ops()); n != 0 {
		t.Fatalf("no banner before Push enters User Mode, got %d ops", n)
	}
	rt.noticeMu.Lock()
	rt.noticeUntil = time.Now().Add(time.Minute)
	rt.noticeMu.Unlock()
	rt.drawUserModeBanner()
	var text string
	for _, op := range rt.frame.Ops() {
		if op.Kind == "text" {
			text = string(op.Params)
		}
	}
	if !strings.Contains(text, "User Mode Active") {
		t.Errorf("banner ops: %v", rt.frame.Ops())
	}
	rt.frame.Reset()
	rt.noticeMu.Lock()
	rt.noticeUntil = time.Now().Add(-time.Second)
	rt.noticeMu.Unlock()
	rt.drawUserModeBanner()
	if n := len(rt.frame.Ops()); n != 0 {
		t.Errorf("banner must go after %s, got %d ops", userModeBannerFor, n)
	}
}

func TestKeepUserModeWithoutAPortIsHarmless(t *testing.T) {
	rt, err := New(nil, nil, Options{KeepUserMode: true}, stubModule{})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetKeepUserMode(true) // no port, so no keeper: must not panic
	if rt.KeepUserMode() {
		t.Error("with no port nothing is kept")
	}
	rt.Shutdown()
}

// idleRig: a Runtime with no hardware and a keeper that is on.
func idleRig(t *testing.T, confirmed bool) *Runtime {
	t.Helper()
	rt, err := New(nil, nil, Options{}, stubModule{})
	if err != nil {
		t.Fatal(err)
	}
	r := newKeeperRig()
	rt.keeper = r.k
	r.k.SetOn(true)
	if confirmed {
		r.k.Seen(pmidi.UserMode)
	}
	return rt
}

func frameText(rt *Runtime) string {
	var b strings.Builder
	for _, op := range rt.frame.Ops() {
		if op.Kind == "text" {
			b.Write(op.Params)
		}
	}
	return b.String()
}

func TestIdleScreenSaysUserModeActiveWhileNoModuleIsActive(t *testing.T) {
	rt := idleRig(t, true)
	if err := rt.drawFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(frameText(rt), "User Mode Active") {
		t.Errorf("idle frame: %s", frameText(rt))
	}
}

func TestIdleScreenAdmitsWhenPushHasNotConfirmed(t *testing.T) {
	rt := idleRig(t, false)
	_ = rt.drawFrame(context.Background())
	got := frameText(rt)
	if strings.Contains(got, "User Mode Active") || !strings.Contains(got, "Entering User Mode") {
		t.Errorf("before the Push answers the screen must not claim User Mode: %s", got)
	}
}

func TestIdleScreenIsCentredAndAscii(t *testing.T) {
	for _, s := range []string{"User Mode Active", "Entering User Mode"} {
		if x := centreX(s, 4); x < 0 {
			t.Errorf("%q at scale 4 does not fit the screen (x=%d)", s, x)
		}
	}
	for _, s := range []string{"User Mode Active", "Entering User Mode", "Pick a module in Push Tethered App.", "Waiting for Push."} {
		for _, r := range s {
			if r < 0x20 || r > 0x7e {
				t.Errorf("non-ASCII rune in %q", s)
			}
		}
	}
}

func TestNoIdleScreenWhenKeepingIsOff(t *testing.T) {
	rt, _ := New(nil, nil, Options{}, stubModule{})
	r := newKeeperRig()
	rt.keeper = r.k // off
	_ = rt.drawFrame(context.Background())
	if n := len(rt.frame.Ops()); n != 0 {
		t.Errorf("an off keeper must draw nothing without a module, got %d ops", n)
	}
}

func TestRunDoesNotStartAModuleByItselfWhileKeepingUserMode(t *testing.T) {
	for _, c := range []struct {
		keep       bool
		wantActive string
	}{{true, ""}, {false, "stub"}} {
		rt, _ := New(nil, nil, Options{KeepUserMode: c.keep, FPS: 100}, stubModule{})
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		_ = rt.Run(ctx)
		cancel()
		if got := rt.Active().ID; got != c.wantActive {
			t.Errorf("keep=%v: active %q, want %q", c.keep, got, c.wantActive)
		}
	}
}

// ledRecorder stands in for the port and records every LED write in order.
type ledRecorder struct{ ops []string }

func (l *ledRecorder) SetPad(note, colour byte) error {
	l.ops = append(l.ops, fmt.Sprintf("pad %d=%d", note, colour))
	return nil
}
func (l *ledRecorder) SetButton(cc, value byte) error {
	l.ops = append(l.ops, fmt.Sprintf("btn %d=%d", cc, value))
	return nil
}
func (l *ledRecorder) ClearAllLEDs() { l.ops = append(l.ops, "clear all") }

func TestEnteringUserModeBlanksLivesColorsThenReplaysTheModule(t *testing.T) {
	rt, err := New(nil, nil, Options{}, stubModule{})
	if err != nil {
		t.Fatal(err)
	}
	rec := &ledRecorder{}
	rt.ledsOverride = rec

	// The active module lights a pad and a button, and turns one off again.
	h := &moduleHost{rt: rt, id: "stub"}
	h.SetPad(40, 7)
	h.SetPad(41, 9)
	h.SetPad(41, 0)
	h.SetButton(85, 126)
	rec.ops = nil

	rt.enteredUserMode() // Live had painted over everything while Push was in Live Mode

	if len(rec.ops) == 0 || rec.ops[0] != "clear all" {
		t.Fatalf("must blank everything first: %v", rec.ops)
	}
	got := strings.Join(rec.ops, ",")
	for _, want := range []string{"pad 40=7", "btn 85=126", "btn 59=120"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "pad 41") {
		t.Errorf("a pad the module turned off must not come back: %s", got)
	}
	if last := rec.ops[len(rec.ops)-1]; last != "btn 59=120" {
		t.Errorf("the User button goes white last, got %q", last)
	}
	rt.noticeMu.Lock()
	banner := time.Now().Before(rt.noticeUntil)
	rt.noticeMu.Unlock()
	if !banner {
		t.Error("entering User Mode must start the banner")
	}
}

func TestSwitchingModuleForgetsTheOldModulesLEDs(t *testing.T) {
	rt, _ := New(nil, nil, Options{}, stubModule{})
	rec := &ledRecorder{}
	rt.ledsOverride = rec
	h := &moduleHost{rt: rt, id: "stub"}
	h.SetPad(40, 7)
	rt.resetLEDShadow() // what a module switch does
	rec.ops = nil
	rt.enteredUserMode()
	if strings.Contains(strings.Join(rec.ops, ","), "pad 40") {
		t.Errorf("an old module's LED came back: %v", rec.ops)
	}
}
