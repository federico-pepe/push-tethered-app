package main

import "testing"

func TestPushPortMatchesOnlyTheNumberedPort(t *testing.T) {
	ports := []string{"#1 Ableton Push 3 Live Port", "#10 Ableton Push 3 User Port"}
	for _, c := range []struct {
		n    int
		name string
		ok   bool
	}{
		{1, "Ableton Push 3 Live Port", true},
		{10, "Ableton Push 3 User Port", true},
		{0, "", false}, // "#0 " must not match "#10 "
		{2, "", false},
	} {
		got, ok := pushPort(ports, c.n)
		if ok != c.ok || got != c.name {
			t.Errorf("port %d: got %q ok=%v, want %q ok=%v", c.n, got, ok, c.name, c.ok)
		}
	}
}
