package host

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// userConfigDir is a seam over os.UserConfigDir so tests can point it at a
// temp directory instead of the real one.
var userConfigDir = os.UserConfigDir

// configDir returns the directory this app's per-module config files live in,
// creating it if needed.
func configDir() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "push-tethered-app", "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// configFilePath returns where a module's config document would live, without
// requiring it to exist yet. Used only for the informational log line on
// activation — modules themselves never see a path, only Store.Get/Set — so
// that a user can find and hand-edit the file before any config UI exists.
func configFilePath(moduleID string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, moduleID+".json"), nil
}

// fileStore is a module.Store backed by one JSON file.
type fileStore struct {
	path string
}

// newStore returns a Store for the given module. If the config directory
// cannot be resolved (no HOME, a locked-down container, ...), it degrades to
// an in-memory store rather than failing module activation over something a
// module's actual job never depends on.
func newStore(moduleID string) module.Store {
	path, err := configFilePath(moduleID)
	if err != nil {
		return memStore{}
	}
	return &fileStore{path: path}
}

// Get unmarshals the stored document into v. Per the module.Store contract,
// no file yet is not an error — v is left with whatever defaults the caller
// set before calling Get.
func (s *fileStore) Get(v any) error {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Set writes v atomically: to a temp file in the same directory, then a
// rename. A crash or a concurrent read can therefore never observe a
// half-written document — the rename is the only thing that makes the new
// content visible.
func (s *fileStore) Set(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// memStore is the degrade path when configDir cannot be resolved: Get is
// always a no-op (nothing stored), Set silently discards. A module still
// functions for the session; it just does not survive a restart.
type memStore struct{}

func (memStore) Get(any) error { return nil }
func (memStore) Set(any) error { return nil }
