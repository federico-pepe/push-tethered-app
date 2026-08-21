package module

import "testing"

func TestExclusiveGroupKeepsOneSelected(t *testing.T) {
	g := NewButtonGroup(true)
	g.Toggle(0)
	if !g.IsSelected(0) {
		t.Fatal("index 0 should be selected after Toggle")
	}
	g.Toggle(2)
	if g.IsSelected(0) {
		t.Error("index 0 should have been cleared when index 2 was selected")
	}
	if !g.IsSelected(2) {
		t.Error("index 2 should be selected")
	}
}

func TestExclusiveGroupReselectIsNoOp(t *testing.T) {
	g := NewButtonGroup(true)
	g.Toggle(1)
	g.Toggle(1)
	if !g.IsSelected(1) {
		t.Error("re-pressing the selected button should leave it selected, not clear the group")
	}
}

func TestIndependentGroupTogglesFreely(t *testing.T) {
	g := NewButtonGroup(false)
	g.Toggle(0)
	g.Toggle(1)
	if !g.IsSelected(0) || !g.IsSelected(1) {
		t.Fatal("independent group should allow multiple selections at once")
	}
	g.Toggle(0)
	if g.IsSelected(0) {
		t.Error("toggling a selected index off should clear it")
	}
	if !g.IsSelected(1) {
		t.Error("toggling index 0 should not affect index 1")
	}
}
