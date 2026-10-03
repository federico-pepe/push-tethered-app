package pushmap

import (
	"slices"
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

func TestLEDButtonsHaveNoEncodersAndKeepEveryButtonLive(t *testing.T) {
	got := LEDButtons()
	for _, enc := range []byte{14, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79} {
		if slices.Contains(got, enc) {
			t.Errorf("encoder CC %d has no LED but is in the list", enc)
		}
	}
	// Buttons Live lights, and the ones this feature lights itself.
	for _, cc := range []byte{push3.CCPlay, push3.CCSession, push3.CCUserMode, push3.CCScreenTop1, push3.CCScreenBot8, push3.CCPageLeft, 36, 43} {
		if !slices.Contains(got, cc) {
			t.Errorf("CC %d is missing from the LED button list", cc)
		}
	}
	if !slices.IsSorted(got) {
		t.Error("list must be sorted")
	}
	seen := map[byte]bool{}
	for _, cc := range got {
		if seen[cc] {
			t.Errorf("CC %d twice", cc)
		}
		seen[cc] = true
	}
}
