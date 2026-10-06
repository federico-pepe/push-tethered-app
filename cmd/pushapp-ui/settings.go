package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// settings are the app-wide preferences that outlive a run. A missing file
// means defaults, so a fresh install needs no setup.
type settings struct {
	// NoUpdateCheck is stored inverted so the zero value (check on) is the
	// default and a missing file or key needs no special case.
	NoUpdateCheck bool `json:"noUpdateCheck,omitempty"`
}

// settingsStore reads and writes settings.json in the app's config
// directory, next to the logs directory.
type settingsStore struct {
	mu sync.Mutex
}

func settingsPath() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding a config directory: %w", err)
	}
	return filepath.Join(base, "push-tethered-app", "settings.json"), nil
}

func (s *settingsStore) load() settings {
	var st settings
	path, err := settingsPath()
	if err != nil {
		return st
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &st) // a corrupt file falls back to defaults
	}
	return st
}

// update applies fn to the current settings and saves the result.
func (s *settingsStore) update(fn func(*settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := settingsPath()
	if err != nil {
		return err
	}
	st := s.load()
	fn(&st)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}
