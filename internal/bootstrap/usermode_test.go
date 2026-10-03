package bootstrap

import (
	"testing"

	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
)

func ref(unit string, cable int, role string, out int) pmidi.PortRef {
	return pmidi.PortRef{Unit: unit, Cable: cable, Role: role, InName: unit + " " + role, OutNum: out}
}

func TestPickUserRefByRole(t *testing.T) {
	refs := []pmidi.PortRef{ref("A", 1, "Live", 0), ref("A", 2, "User", 1), ref("A", 3, "External", 2)}
	got, err := pickUserRef(refs, refs[0])
	if err != nil || got.Cable != 2 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestPickUserRefOnWindowsUsesTheCablePosition(t *testing.T) {
	// WinMM has no jack strings: every role is empty.
	refs := []pmidi.PortRef{ref("W", 1, "", 0), ref("W", 2, "", 1), ref("W", 3, "", 2)}
	got, err := pickUserRef(refs, refs[0])
	if err != nil || got.Cable != 2 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestPickUserRefStaysInsideTheUnit(t *testing.T) {
	refs := []pmidi.PortRef{
		ref("A", 1, "Live", 0), ref("B", 1, "Live", 3), ref("B", 2, "User", 4), ref("A", 2, "User", 1),
	}
	got, err := pickUserRef(refs, refs[1])
	if err != nil || got.Unit != "B" {
		t.Errorf("unit B's main port got %+v, %v", got, err)
	}
}

func TestPickUserRefNeedsAnOutputCable(t *testing.T) {
	refs := []pmidi.PortRef{ref("A", 1, "Live", 0), ref("A", 2, "User", -1)}
	if _, err := pickUserRef(refs, refs[0]); err == nil {
		t.Error("a User Port without output must be refused: the switch is sent on it")
	}
}

func TestPickUserRefSkipsAmbiguousCablesAndMissingOnes(t *testing.T) {
	amb := ref("A", 2, "User", -1)
	amb.Ambiguous = true
	if _, err := pickUserRef([]pmidi.PortRef{ref("A", 1, "Live", 0), amb}, ref("A", 1, "Live", 0)); err == nil {
		t.Error("an ambiguous cable must not be guessed")
	}
	if _, err := pickUserRef([]pmidi.PortRef{ref("A", 1, "Live", 0)}, ref("A", 1, "Live", 0)); err == nil {
		t.Error("no User Port must be an error")
	}
}

func TestPortRefIsUser(t *testing.T) {
	for _, c := range []struct {
		r    pmidi.PortRef
		want bool
	}{
		{ref("A", 2, "User", 1), true},
		{ref("A", 1, "Live", 0), false},
		{ref("W", 2, "", 1), true},
		{ref("W", 1, "", 0), false},
		{ref("A", 3, "External", 2), false},
	} {
		if c.r.IsUser() != c.want {
			t.Errorf("%+v: got %v", c.r, !c.want)
		}
	}
}
