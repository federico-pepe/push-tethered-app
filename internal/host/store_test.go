package host

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempConfigDir points userConfigDir at a fresh temp directory for the
// duration of a test, so store tests never touch the real OS config location.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = prev })
	return dir
}

type doc struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestStoreRoundTrips(t *testing.T) {
	withTempConfigDir(t)
	s := newStore("thing", "")

	if err := s.Set(doc{Name: "a", N: 1}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var got doc
	if err := s.Get(&got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != (doc{Name: "a", N: 1}) {
		t.Errorf("Get = %+v, want {a 1}", got)
	}
}

// TestStoreMissingFileLeavesDefaults pins the module.Store contract: nothing
// stored yet is not an error, and the caller's pre-set defaults must survive.
func TestStoreMissingFileLeavesDefaults(t *testing.T) {
	withTempConfigDir(t)
	s := newStore("never-saved", "")

	got := doc{Name: "default", N: 42}
	if err := s.Get(&got); err != nil {
		t.Fatalf("Get on a missing file returned an error: %v", err)
	}
	if got != (doc{Name: "default", N: 42}) {
		t.Errorf("Get on a missing file changed the defaults: %+v", got)
	}
}

// TestStoreIsPerModule — two modules must never see each other's document.
func TestStoreIsPerModule(t *testing.T) {
	withTempConfigDir(t)
	a := newStore("module-a", "")
	b := newStore("module-b", "")

	if err := a.Set(doc{Name: "a's data"}); err != nil {
		t.Fatal(err)
	}
	var got doc
	if err := b.Get(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name == "a's data" {
		t.Error("module b read module a's stored document")
	}
}

// TestStoreIsPerDevice — the same module active on two different Push units
// (pushapp-ui running two sessions) must not share one config document.
func TestStoreIsPerDevice(t *testing.T) {
	withTempConfigDir(t)
	a := newStore("seq", "serial:AAA")
	b := newStore("seq", "serial:BBB")

	if err := a.Set(doc{Name: "unit a's data"}); err != nil {
		t.Fatal(err)
	}
	var got doc
	if err := b.Get(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name == "unit a's data" {
		t.Error("unit b read unit a's stored document")
	}

	pathA, err := configFilePath("seq", "serial:AAA")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(pathA) != "seq-serial_AAA.json" {
		t.Errorf("config path = %s, want seq-serial_AAA.json", filepath.Base(pathA))
	}
}

// TestStoreSetIsAtomic checks the write goes through a temp file and rename,
// so a reader never sees a half-written document.
func TestStoreSetIsAtomic(t *testing.T) {
	dir := withTempConfigDir(t)
	s := newStore("atomic", "")
	if err := s.Set(doc{Name: "x"}); err != nil {
		t.Fatal(err)
	}

	path, err := configFilePath("atomic", "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(dir, "push-tethered-app", "modules") {
		t.Errorf("config path = %s, in an unexpected directory", path)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("a .tmp file was left behind after Set — rename did not clean up")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("final file does not exist: %v", err)
	}
}

func TestStoreOverwrites(t *testing.T) {
	withTempConfigDir(t)
	s := newStore("thing", "")

	_ = s.Set(doc{Name: "first", N: 1})
	_ = s.Set(doc{Name: "second", N: 2})

	var got doc
	if err := s.Get(&got); err != nil {
		t.Fatal(err)
	}
	if got != (doc{Name: "second", N: 2}) {
		t.Errorf("Get after two Sets = %+v, want the second value", got)
	}
}

// TestConfigDirFailureDegradesToMemStore — a module must still activate even
// when no config directory can be resolved; persistence is not load-bearing
// for a module's actual job.
func TestConfigDirFailureDegradesToMemStore(t *testing.T) {
	prev := userConfigDir
	userConfigDir = func() (string, error) { return "", os.ErrPermission }
	t.Cleanup(func() { userConfigDir = prev })

	s := newStore("whatever", "")
	if _, ok := s.(memStore); !ok {
		t.Fatalf("newStore with a failing userConfigDir = %T, want memStore", s)
	}
	// And it must still behave like a store: Set then Get should not error,
	// even though nothing actually persists.
	if err := s.Set(doc{Name: "x"}); err != nil {
		t.Errorf("memStore.Set returned an error: %v", err)
	}
	got := doc{Name: "default"}
	if err := s.Get(&got); err != nil {
		t.Errorf("memStore.Get returned an error: %v", err)
	}
	if got.Name != "default" {
		t.Error("memStore.Get should leave defaults untouched")
	}
}
