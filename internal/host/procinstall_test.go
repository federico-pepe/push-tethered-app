package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/federico-pepe/push-tethered-app/internal/host/procmod"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/module/moduletest"
)

// fakeModule is a minimal compiled-in module.Module for tests that need one
// registered in Runtime.modules but never actually activate it.
type fakeModule struct{ id string }

func (m fakeModule) Meta() module.Meta    { return module.Meta{ID: m.id, Name: m.id} }
func (fakeModule) Init(module.Host) error { return nil }
func (fakeModule) Handle(module.Event)    {}
func (fakeModule) Draw(*module.Frame)     {}
func (fakeModule) Close() error           { return nil }

var _ module.Module = fakeModule{}

// writeModuleDir creates a self-contained module source directory — the
// shape Install expects to copy — without anything that actually needs to
// run, since these tests exercise Install/Uninstall/List/findModule, not the
// process supervisor itself (that's procmod's own test suite).
func writeModuleDir(t *testing.T, id string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"id":"` + id + `","name":"` + id + `","exec":"true"}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstallRegistersAndCopiesFiles(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{}

	src := writeModuleDir(t, "hello")
	meta, err := rt.Install(src)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if meta.ID != "hello" {
		t.Errorf("Meta.ID = %q, want %q", meta.ID, "hello")
	}

	root, _ := procmod.InstalledRoot()
	if _, err := os.Stat(filepath.Join(root, "hello", "manifest.json")); err != nil {
		t.Errorf("installed manifest not found at expected location: %v", err)
	}

	if rt.findModule("hello") == nil {
		t.Error("findModule did not find the installed module")
	}
}

func TestInstallRefusesDuplicateID(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{modules: []module.Module{fakeModule{id: "monitor"}}}

	// Collides with a compiled-in module's ID.
	if _, err := rt.Install(writeModuleDir(t, "monitor")); err == nil {
		t.Error("Install did not refuse an ID colliding with a compiled-in module")
	}

	// Collides with an already-installed module's ID.
	if _, err := rt.Install(writeModuleDir(t, "hello")); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if _, err := rt.Install(writeModuleDir(t, "hello")); err == nil {
		t.Error("Install did not refuse an ID already installed")
	}
}

func TestInstallLeavesSourceDirUntouched(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{}

	src := writeModuleDir(t, "hello")
	if _, err := rt.Install(src); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "manifest.json")); err != nil {
		t.Errorf("source directory was modified/removed: %v", err)
	}
}

func TestUninstallRemovesFilesAndRegistration(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{}

	if _, err := rt.Install(writeModuleDir(t, "hello")); err != nil {
		t.Fatal(err)
	}
	root, _ := procmod.InstalledRoot()
	dir := filepath.Join(root, "hello")

	if err := rt.Uninstall("hello"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if rt.findModule("hello") != nil {
		t.Error("findModule still finds the module after Uninstall")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("installed directory still exists after Uninstall: %v", err)
	}
}

func TestUninstallRefusesActiveModule(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{}
	if _, err := rt.Install(writeModuleDir(t, "hello")); err != nil {
		t.Fatal(err)
	}
	rt.active = rt.findModule("hello")

	if err := rt.Uninstall("hello"); err == nil {
		t.Error("Uninstall did not refuse the active module")
	}
	if rt.findModule("hello") == nil {
		t.Error("Uninstall removed the module despite refusing")
	}
}

func TestUninstallRefusesBuiltIn(t *testing.T) {
	rt := &Runtime{modules: []module.Module{fakeModule{id: "monitor"}}}
	if err := rt.Uninstall("monitor"); err == nil {
		t.Error("Uninstall did not refuse a compiled-in module")
	}
}

// TestIsInstalled is what the UI uses to decide whether an Uninstall control
// makes sense for a given module at all.
func TestIsInstalled(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{modules: []module.Module{fakeModule{id: "monitor"}}}

	if rt.IsInstalled("monitor") {
		t.Error("IsInstalled(monitor) = true, want false (compiled-in)")
	}
	if rt.IsInstalled("does-not-exist") {
		t.Error("IsInstalled on an unknown id = true, want false")
	}

	if _, err := rt.Install(writeModuleDir(t, "hello")); err != nil {
		t.Fatal(err)
	}
	if !rt.IsInstalled("hello") {
		t.Error("IsInstalled(hello) = false after Install, want true")
	}
}

func TestUninstallUnknownID(t *testing.T) {
	rt := &Runtime{}
	if err := rt.Uninstall("does-not-exist"); err == nil {
		t.Error("Uninstall did not error for an unknown id")
	}
}

func TestListMergesCompiledInAndInstalled(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{modules: []module.Module{fakeModule{id: "monitor"}, fakeModule{id: "thru"}}}
	if _, err := rt.Install(writeModuleDir(t, "hello")); err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, m := range rt.List() {
		got[m.ID] = true
	}
	for _, want := range []string{"monitor", "thru", "hello"} {
		if !got[want] {
			t.Errorf("List() missing %q: %v", want, rt.List())
		}
	}
}

func TestLoadInstalledScansExistingDirectories(t *testing.T) {
	root := t.TempDir()
	defer procmod.SetInstalledRootForTest(root)()

	// Simulate a module installed by a previous run: files already in place,
	// no copy step this time.
	dir := filepath.Join(root, "hello")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"),
		[]byte(`{"id":"hello","name":"Hello","exec":"true"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{}
	if err := rt.LoadInstalled(); err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if rt.findModule("hello") == nil {
		t.Error("LoadInstalled did not register the pre-existing module")
	}
}

func TestLoadInstalledSkipsBrokenManifestsWithoutFailing(t *testing.T) {
	root := t.TempDir()
	defer procmod.SetInstalledRootForTest(root)()

	broken := filepath.Join(root, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "manifest.json"), []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(root, "good")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "manifest.json"),
		[]byte(`{"id":"good","name":"Good","exec":"true"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{}
	if err := rt.LoadInstalled(); err != nil {
		t.Fatalf("LoadInstalled: %v (a broken module should be skipped, not fatal)", err)
	}
	if rt.findModule("good") == nil {
		t.Error("LoadInstalled did not register the valid module alongside the broken one")
	}
	if rt.findModule("broken") != nil {
		t.Error("LoadInstalled somehow registered a module with an invalid manifest")
	}
}

func TestLoadInstalledWithNothingInstalledIsNotAnError(t *testing.T) {
	// Note: procmod.InstalledRoot() creates the directory as a side effect of
	// resolving it, so this exercises the "empty directory" case in practice
	// rather than a genuinely missing one — LoadInstalled's os.ErrNotExist
	// branch is defensive for the narrower case of the directory vanishing
	// between InstalledRoot() and the read. Either way, no modules installed
	// yet must not be an error.
	defer procmod.SetInstalledRootForTest(filepath.Join(t.TempDir(), "fresh"))()
	rt := &Runtime{}
	if err := rt.LoadInstalled(); err != nil {
		t.Errorf("LoadInstalled with nothing installed = %v, want nil", err)
	}
}

func TestUpdateReplacesRegisteredModule(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{}

	if _, err := rt.Install(writeModuleDir(t, "hello")); err != nil {
		t.Fatal(err)
	}

	newSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(newSrc, "manifest.json"),
		[]byte(`{"id":"hello","name":"Hello","version":"2.0.0","exec":"true"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := rt.Update("hello", newSrc)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if meta.Version != "2.0.0" {
		t.Errorf("Version = %q, want 2.0.0", meta.Version)
	}
	if rt.findModule("hello").Meta().Version != "2.0.0" {
		t.Error("findModule still returns the pre-update registration")
	}
}

func TestUpdateRefusesActiveModule(t *testing.T) {
	defer procmod.SetInstalledRootForTest(t.TempDir())()
	rt := &Runtime{}
	if _, err := rt.Install(writeModuleDir(t, "hello")); err != nil {
		t.Fatal(err)
	}
	rt.active = rt.findModule("hello")

	if _, err := rt.Update("hello", writeModuleDir(t, "hello")); err == nil {
		t.Error("Update did not refuse the active module")
	}
}

// Sanity check that moduletest.Host still satisfies module.Host — used
// nowhere in this file directly, but if this ever stops compiling it means
// the ABI moved without the fake keeping up, which would silently invalidate
// every module's tests.
var _ module.Host = (*moduletest.Host)(nil)
