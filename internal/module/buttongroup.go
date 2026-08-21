package module

// ButtonGroup tracks selection state for a cluster of soft-buttons drawn
// with the same widgets.SoftButton.Group id.
//
// It is a plain state helper, not part of the host ABI. Soft-buttons have
// no physical per-button LED — their state feedback *is* the on-screen
// BotStrip label (SoftButtonState's color), so lighting a group needs no
// Host method: a module just re-renders its BotStrip from this state on
// the next Draw, the same as any other UI state change.
type ButtonGroup struct {
	// Exclusive selects radio behavior: Toggle always leaves exactly one
	// index selected. false gives independent toggles, each on/off on its
	// own — e.g. per-track mute buttons.
	Exclusive bool

	selected map[int]bool
}

// NewButtonGroup returns an empty group. Call Toggle from Handle as
// buttons are pressed, then IsSelected from Draw to build the frame's
// [8]SoftButton state.
func NewButtonGroup(exclusive bool) *ButtonGroup {
	return &ButtonGroup{Exclusive: exclusive, selected: map[int]bool{}}
}

// Toggle flips button i's selection. In an exclusive group this selects i
// and clears every other index — pressing the already-selected button is a
// no-op, matching ordinary radio-button behavior rather than allowing the
// group to end up with nothing selected.
func (g *ButtonGroup) Toggle(i int) {
	if g.Exclusive {
		for k := range g.selected {
			delete(g.selected, k)
		}
		g.selected[i] = true
		return
	}
	if g.selected[i] {
		delete(g.selected, i)
	} else {
		g.selected[i] = true
	}
}

// IsSelected reports whether button i is currently selected.
func (g *ButtonGroup) IsSelected(i int) bool { return g.selected[i] }
