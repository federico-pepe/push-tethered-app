package main

import "testing"

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	defer func() { userConfigDir = old }()

	var s settingsStore
	if s.load().NoUpdateCheck {
		t.Fatal("default must have the check on")
	}
	if err := s.update(func(st *settings) { st.NoUpdateCheck = true }); err != nil {
		t.Fatal(err)
	}
	if !s.load().NoUpdateCheck {
		t.Fatal("setting not saved")
	}
	if err := s.update(func(st *settings) { st.NoUpdateCheck = false }); err != nil {
		t.Fatal(err)
	}
	if s.load().NoUpdateCheck {
		t.Fatal("setting not cleared")
	}
}
