package padpointer

import (
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/module/moduletest"
)

func newTest(t *testing.T) (*Module, *moduletest.Host) {
	t.Helper()
	h := &moduletest.Host{}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return m, h
}

func TestPadPressMovesCursorToTheRightRow(t *testing.T) {
	m, _ := newTest(t)

	// Physical top row (7) must land on item 0, the top of the on-screen menu.
	m.Handle(module.Pad{Note: 92, Col: 0, Row: 7, Channel: 1, Velocity: 40, Pressed: true})
	if m.cursor != 0 {
		t.Errorf("cursor after top-row press = %d, want 0", m.cursor)
	}

	// Physical bottom row (0) must land on item 7, the bottom of the menu.
	m.Handle(module.Pad{Note: 36, Col: 0, Row: 0, Channel: 1, Velocity: 40, Pressed: true})
	if m.cursor != 7 {
		t.Errorf("cursor after bottom-row press = %d, want 7", m.cursor)
	}
}

func TestReleaseClearsHeldButKeepsCursor(t *testing.T) {
	m, _ := newTest(t)

	m.Handle(module.Pad{Note: 44, Col: 1, Row: 1, Channel: 1, Velocity: 40, Pressed: true})
	m.Handle(module.Pad{Note: 44, Col: 1, Row: 1, Channel: 1, Pressed: false})

	if m.holding {
		t.Error("holding should be false after release")
	}
	if m.cursor != rowToItem(1) {
		t.Error("cursor should stay on the last-pressed row after release")
	}
}

func TestLowPressureDoesNotClick(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Pad{Note: 44, Col: 1, Row: 1, Channel: 1, Velocity: 40, Pressed: true})
	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: clickThreshold - 1})

	if m.checked[m.cursor] {
		t.Error("pressure below threshold must not toggle the item")
	}
	if !m.holding {
		t.Error("a sub-threshold pressure reading must not clear holding")
	}
}

func TestThresholdPressureClicks(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Pad{Note: 44, Col: 1, Row: 1, Channel: 1, Velocity: 40, Pressed: true})
	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: clickThreshold})

	if !m.checked[m.cursor] {
		t.Error("pressure at/above threshold must toggle the item")
	}
	if m.holding {
		t.Error("a click should clear holding so it does not re-fire while still pressed")
	}
	if m.lastMsg == "" {
		t.Error("a click should set a status message")
	}
}

func TestExpressionIgnoredWhenNotHolding(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: 127})

	for i, c := range m.checked {
		if c {
			t.Errorf("item %d toggled with no pad held", i)
		}
	}
}

func TestDrawDoesNotPanicFromFreshInit(t *testing.T) {
	m, _ := newTest(t)
	f := module.NewFrame(960, 160)
	m.Draw(f)
	if f.Failed() != 0 {
		t.Errorf("Draw recorded %d failed ops", f.Failed())
	}
}

func TestDPadRightSwitchesToCrosshairPage(t *testing.T) {
	m, _ := newTest(t)
	if m.page != 0 {
		t.Fatalf("initial page = %d, want 0 (menu)", m.page)
	}
	m.Handle(module.Button{CC: push3.CCDPadRight, Pressed: true})
	if m.page != 1 {
		t.Errorf("page after D-Pad right = %d, want 1 (crosshair)", m.page)
	}
	m.Handle(module.Button{CC: push3.CCDPadLeft, Pressed: true})
	if m.page != 0 {
		t.Errorf("page after D-Pad left = %d, want 0 (menu)", m.page)
	}
}

func TestDPadWrapsAround(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Button{CC: push3.CCDPadLeft, Pressed: true})
	if m.page != numPages-1 {
		t.Errorf("page after wrapping left from 0 = %d, want %d", m.page, numPages-1)
	}
}

func TestCrosshairPageLightTouchMovesCursorButDoesNotAnimate(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1

	if m.haveCursor {
		t.Fatal("haveCursor should be false before any press")
	}
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Pressed: true})

	if !m.haveCursor {
		t.Error("haveCursor should be true after a press")
	}
	if m.cursorCol != 5 || m.cursorRow != 2 {
		t.Errorf("cursor = (%d,%d), want (5,2)", m.cursorCol, m.cursorRow)
	}
	if m.animFrame < animFrames {
		t.Errorf("a touch below the click threshold must not start the animation, animFrame = %d", m.animFrame)
	}
}

func TestCrosshairPageFirmPressTriggersAnimation(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 1, Pressed: true})

	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: clickThreshold - 1})
	if m.animFrame < animFrames {
		t.Errorf("sub-threshold pressure must not trigger the animation, animFrame = %d", m.animFrame)
	}

	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: clickThreshold})
	if m.animFrame != 0 {
		t.Errorf("threshold pressure should start the animation, animFrame = %d, want 0", m.animFrame)
	}
}

func TestCrosshairPageFiresOnlyOncePerHold(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 1, Pressed: true})
	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: clickThreshold})

	// Advance the animation partway, then send more high-pressure readings
	// (realistic: Expression is high-rate) — none of them should restart it.
	m.animFrame = 5
	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: 127})
	if m.animFrame != 5 {
		t.Errorf("a second high-pressure reading in the same hold restarted the animation, animFrame = %d, want 5", m.animFrame)
	}

	// Releasing and pressing again must allow it to fire once more.
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 1, Pressed: false})
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 1, Pressed: true})
	m.Handle(module.Expression{Channel: 1, Kind: "pressure", Value: clickThreshold})
	if m.animFrame != 0 {
		t.Errorf("a new hold should be able to trigger the animation again, animFrame = %d, want 0", m.animFrame)
	}
}

func TestCrosshairPageIgnoresMenuPagePad(t *testing.T) {
	m, _ := newTest(t)
	// Stay on the menu page (page 0): a pad press must not touch crosshair state.
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Pressed: true})
	if m.haveCursor {
		t.Error("a menu-page pad press should not set crosshair state")
	}
}

func TestCrosshairPadOnMPEChannelEnablesFinePosition(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 5, Pressed: true})

	if !m.crosshairMPE {
		t.Error("a pad on an MPE member channel (5) should enable fine positioning")
	}
	if m.crosshairChan != 5 {
		t.Errorf("crosshairChan = %d, want 5", m.crosshairChan)
	}
	if m.crosshairBend != bendCenter || m.crosshairSlide != slideCenter {
		t.Errorf("bend/slide should reset to center on a fresh press, got bend=%d slide=%d", m.crosshairBend, m.crosshairSlide)
	}
}

func TestCrosshairPadOnChannel1IsNotMPE(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 1, Pressed: true})

	if m.crosshairMPE {
		t.Error("a pad on channel 1 (no MPE) must not enable fine positioning")
	}
}

func TestCrosshairExpressionUpdatesBendAndSlideWhenMPE(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 5, Pressed: true})

	m.Handle(module.Expression{Channel: 5, Kind: "bend", Value: 12000})
	m.Handle(module.Expression{Channel: 5, Kind: "slide", Value: 100})

	if m.crosshairBend != 12000 {
		t.Errorf("crosshairBend = %d, want 12000", m.crosshairBend)
	}
	if m.crosshairSlide != 100 {
		t.Errorf("crosshairSlide = %d, want 100", m.crosshairSlide)
	}
}

func TestCrosshairExpressionIgnoredOnWrongChannel(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 5, Pressed: true})

	// A stray Expression from a different pad's MPE channel must not leak in.
	m.Handle(module.Expression{Channel: 6, Kind: "bend", Value: 12000})

	if m.crosshairBend != bendCenter {
		t.Errorf("crosshairBend changed from a non-matching channel, got %d, want %d", m.crosshairBend, bendCenter)
	}
}

func TestCrosshairBendSlideIgnoredWithoutMPE(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	// Channel 1: no MPE, coarse fallback only.
	m.Handle(module.Pad{Note: 60, Col: 5, Row: 2, Channel: 1, Pressed: true})
	m.Handle(module.Expression{Channel: 1, Kind: "bend", Value: 12000})

	if m.crosshairBend != bendCenter {
		t.Errorf("bend must not apply on a non-MPE hold, got %d, want %d", m.crosshairBend, bendCenter)
	}
}

func TestBendCalibrationWidensButNeverShrinks(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	if m.bendMin != bendCenter || m.bendMax != bendCenter {
		t.Fatalf("fresh module bendMin/bendMax = %d/%d, want both %d", m.bendMin, m.bendMax, bendCenter)
	}

	m.Handle(module.Pad{Note: 60, Col: 3, Row: 2, Channel: 5, Pressed: true})
	m.Handle(module.Expression{Channel: 5, Kind: "bend", Value: 9000})
	m.Handle(module.Expression{Channel: 5, Kind: "bend", Value: 7000})
	if m.bendMax != 9000 {
		t.Errorf("bendMax = %d, want 9000", m.bendMax)
	}
	if m.bendMin != 7000 {
		t.Errorf("bendMin = %d, want 7000", m.bendMin)
	}

	// A reading back toward center must not shrink the calibrated range.
	m.Handle(module.Expression{Channel: 5, Kind: "bend", Value: 8200})
	if m.bendMax != 9000 || m.bendMin != 7000 {
		t.Errorf("calibration shrank: bendMin/bendMax = %d/%d, want unchanged 7000/9000", m.bendMin, m.bendMax)
	}
}

func TestCrosshairAnimationAdvancesThenStops(t *testing.T) {
	m, _ := newTest(t)
	m.page = 1
	m.Handle(module.Pad{Note: 60, Col: 0, Row: 0, Pressed: true})

	f := module.NewFrame(960, 160)
	for i := 0; i < animFrames; i++ {
		m.Draw(f)
		f.Reset()
	}
	if m.animFrame < animFrames {
		t.Errorf("animFrame after %d draws = %d, want >= %d (animation finished)", animFrames, m.animFrame, animFrames)
	}
}
