package uidemo

import (
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/module/moduletest"
	"github.com/federico-pepe/push-tethered-app/internal/renderframe"
)

func newTest(t *testing.T) (*Module, *moduletest.Host) {
	t.Helper()
	h := &moduletest.Host{Ops: renderframe.SupportedOps()}
	m := New()
	if err := m.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return m, h
}

func TestDPadChangesPage(t *testing.T) {
	m, _ := newTest(t)
	if m.page != 0 {
		t.Fatalf("starts on page %d, want 0", m.page)
	}
	m.Handle(module.Button{CC: push3.CCDPadRight, Pressed: true})
	if m.page != 1 {
		t.Errorf("page after D-Pad right = %d, want 1", m.page)
	}
	m.Handle(module.Button{CC: push3.CCDPadLeft, Pressed: true})
	if m.page != 0 {
		t.Errorf("page after D-Pad left = %d, want 0", m.page)
	}
}

func TestPageWrapsBothWays(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Button{CC: push3.CCDPadLeft, Pressed: true})
	if m.page != numPages-1 {
		t.Errorf("page after wrapping left from 0 = %d, want %d", m.page, numPages-1)
	}
	m.Handle(module.Button{CC: push3.CCDPadRight, Pressed: true})
	if m.page != 0 {
		t.Errorf("page after one more D-Pad right = %d, want 0", m.page)
	}
}

func TestDPadReleaseDoesNothing(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Button{CC: push3.CCDPadRight, Pressed: false})
	if m.page != 0 {
		t.Errorf("a release event should not change the page, got %d", m.page)
	}
}

func TestPadTogglesGridAndLightsLED(t *testing.T) {
	m, h := newTest(t)
	note := push3.PadNote(2, 3)
	m.Handle(module.Pad{Note: note, Col: 2, Row: 3, Pressed: true})
	if !m.pads[3][2] {
		t.Error("pad press should toggle pads[row][col] on")
	}
	lit := h.LitPads()
	if lit[note] == 0 {
		t.Error("pad press should light its own LED")
	}

	m.Handle(module.Pad{Note: note, Col: 2, Row: 3, Pressed: true})
	if m.pads[3][2] {
		t.Error("second press should toggle it back off")
	}
}

func TestSoftButtonExclusiveGroup(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Button{CC: push3.CCScreenBotN(0), Pressed: true})
	if !m.quantize.IsSelected(0) {
		t.Fatal("soft button 0 should select index 0 in the exclusive group")
	}
	m.Handle(module.Button{CC: push3.CCScreenBotN(1), Pressed: true})
	if m.quantize.IsSelected(0) {
		t.Error("selecting index 1 should have cleared index 0 (exclusive group)")
	}
	if !m.quantize.IsSelected(1) {
		t.Error("index 1 should now be selected")
	}
}

func TestSoftButtonIndependentGroup(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Button{CC: push3.CCScreenBotN(4), Pressed: true})
	m.Handle(module.Button{CC: push3.CCScreenBotN(5), Pressed: true})
	if !m.toggles.IsSelected(4) || !m.toggles.IsSelected(5) {
		t.Error("independent group should allow both to be selected at once")
	}
}

func TestEncoderAccumulates(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Encoder{Index: 0, Delta: 3})
	m.Handle(module.Encoder{Index: 0, Delta: -1})
	if m.enc[0] != 2 {
		t.Errorf("enc[0] = %d, want 2", m.enc[0])
	}
	m.Handle(module.Encoder{Index: 99, Delta: 5}) // out of range, must not panic or write elsewhere
}

// TestEncoderStopsAtLimitAndReversesImmediately guards the endless-encoder
// contract: turning past the max pins the value at 100 instead of wrapping,
// and turning back immediately decreases it — it must not have to unwind an
// accumulator that kept climbing past the limit.
func TestEncoderStopsAtLimitAndReversesImmediately(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Encoder{Index: 0, Delta: 500}) // way past the 0-100 range
	if m.enc[0] != 100 {
		t.Errorf("enc[0] = %d, want 100 (clamped at max, not wrapped)", m.enc[0])
	}
	m.Handle(module.Encoder{Index: 0, Delta: -1})
	if m.enc[0] != 99 {
		t.Errorf("enc[0] = %d, want 99 (reversal should respond immediately)", m.enc[0])
	}
}

func TestClampedFracClampsAndStaysInRange(t *testing.T) {
	for _, raw := range []int{0, 50, 99, 100, 150, -1, -50, -150} {
		f := clampedFrac(raw)
		if f < 0 || f > 1 {
			t.Errorf("clampedFrac(%d) = %v, want in [0,1]", raw, f)
		}
	}
	// An endless encoder stops at the control's limit rather than wrapping
	// past it: going over 100 or under 0 pins at 1 or 0, it doesn't roll
	// back around.
	if f := clampedFrac(150); f != 1 {
		t.Errorf("clampedFrac(150) = %v, want 1 (clamped at max, not wrapped)", f)
	}
	if f := clampedFrac(-50); f != 0 {
		t.Errorf("clampedFrac(-50) = %v, want 0 (clamped at min, not wrapped)", f)
	}
}

// TestEveryPageDrawsOnlySupportedOps walks every page and checks Draw
// never emits an op the host can't render — the same guard every other
// module's Draw test uses, run once per page here since each page calls
// different Frame methods.
func TestEveryPageDrawsOnlySupportedOps(t *testing.T) {
	m, h := newTest(t)
	supported := map[string]bool{}
	for _, k := range h.SupportedOps() {
		supported[k] = true
	}

	for p := range numPages {
		m.page = p
		f := module.NewFrame(960, 160)
		m.Draw(f)
		if len(f.Ops()) == 0 {
			t.Errorf("page %d (%s): Draw emitted no ops", p, pageNames[p])
		}
		if f.Failed() != 0 {
			t.Errorf("page %d (%s): Draw produced %d unmarshalable ops", p, pageNames[p], f.Failed())
		}
		for _, op := range f.Ops() {
			if !supported[op.Kind] {
				t.Errorf("page %d (%s): emitted unsupported op %q", p, pageNames[p], op.Kind)
			}
		}
	}
}

// TestEveryPageDrawsASCIIOnly is TestDrawTextIsASCII's pattern, run across
// every page and with some hardware state exercised first so button
// labels/group state feed real strings, not just zero values.
func TestEveryPageDrawsASCIIOnly(t *testing.T) {
	m, _ := newTest(t)
	m.Handle(module.Button{CC: push3.CCScreenBotN(0), Pressed: true})
	m.Handle(module.Button{CC: push3.CCScreenBotN(4), Pressed: true})
	m.Handle(module.Pad{Note: push3.PadNote(0, 0), Col: 0, Row: 0, Pressed: true})
	for i := range 8 {
		m.Handle(module.Encoder{Index: i, Delta: i * 7})
	}

	for p := range numPages {
		m.page = p
		f := module.NewFrame(960, 160)
		m.Draw(f)
		if bad := moduletest.NonASCIIStrings(f); len(bad) != 0 {
			t.Errorf("page %d (%s): Draw emitted non-ASCII text: %q", p, pageNames[p], bad)
		}
	}
}

func TestMetaHasNoMIDIRequirement(t *testing.T) {
	meta := New().Meta()
	if meta.NeedsMIDIOut || meta.NeedsMIDIIn {
		t.Error("ui-demo is a pure UI/control test — it should need no MIDI port")
	}
	if meta.ID == "" || meta.Name == "" {
		t.Error("Meta must have an ID and Name")
	}
}
