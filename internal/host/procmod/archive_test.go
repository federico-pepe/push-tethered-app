package procmod

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func writeTarGzFile(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()

	path := filepath.Join(t.TempDir(), "module.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstallFromPathAcceptsDirectory(t *testing.T) {
	defer SetInstalledRootForTest(t.TempDir())()
	dir := t.TempDir()
	writeManifest(t, dir, `{"id":"hello","name":"Hello","exec":"true"}`)

	man, err := InstallFromPath(dir)
	if err != nil {
		t.Fatalf("InstallFromPath: %v", err)
	}
	if man.ID != "hello" {
		t.Errorf("ID = %q, want hello", man.ID)
	}
}

func TestInstallFromPathAcceptsTarGz(t *testing.T) {
	defer SetInstalledRootForTest(t.TempDir())()
	archive := writeTarGzFile(t, map[string]string{
		"manifest.json": `{"id":"hello","name":"Hello","exec":"true"}`,
		"run.py":        "print('hi')",
	})

	man, err := InstallFromPath(archive)
	if err != nil {
		t.Fatalf("InstallFromPath: %v", err)
	}
	if man.ID != "hello" {
		t.Errorf("ID = %q, want hello", man.ID)
	}

	root, _ := InstalledRoot()
	if _, err := os.Stat(filepath.Join(root, "hello", "run.py")); err != nil {
		t.Errorf("run.py not installed: %v", err)
	}
}

func TestInstallFromPathAcceptsWrappedTarGz(t *testing.T) {
	defer SetInstalledRootForTest(t.TempDir())()
	archive := writeTarGzFile(t, map[string]string{
		"hello-v1.0.0/manifest.json": `{"id":"hello","name":"Hello","exec":"true"}`,
	})

	man, err := InstallFromPath(archive)
	if err != nil {
		t.Fatalf("InstallFromPath: %v", err)
	}
	if man.ID != "hello" {
		t.Errorf("ID = %q, want hello", man.ID)
	}
}

func TestUpdateReplacesFilesAndKeepsIDMatch(t *testing.T) {
	defer SetInstalledRootForTest(t.TempDir())()
	dir := t.TempDir()
	writeManifest(t, dir, `{"id":"hello","name":"Hello","version":"1.0.0","exec":"true"}`)
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}

	newDir := t.TempDir()
	writeManifest(t, newDir, `{"id":"hello","name":"Hello","version":"1.1.0","exec":"true"}`)
	if err := os.WriteFile(filepath.Join(newDir, "new-file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	man, err := Update("hello", newDir)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if man.Version != "1.1.0" {
		t.Errorf("Version = %q, want 1.1.0", man.Version)
	}

	root, _ := InstalledRoot()
	if _, err := os.Stat(filepath.Join(root, "hello", "new-file.txt")); err != nil {
		t.Errorf("updated file missing: %v", err)
	}
}

func TestUpdateRefusesMismatchedID(t *testing.T) {
	defer SetInstalledRootForTest(t.TempDir())()
	dir := t.TempDir()
	writeManifest(t, dir, `{"id":"hello","name":"Hello","exec":"true"}`)
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}

	otherDir := t.TempDir()
	writeManifest(t, otherDir, `{"id":"other","name":"Other","exec":"true"}`)

	if _, err := Update("hello", otherDir); err == nil {
		t.Error("Update did not refuse a mismatched manifest ID")
	}

	// The original install must survive an aborted update.
	man, err := LoadManifest(filepath.Join(mustInstalledRoot(t), "hello"))
	if err != nil {
		t.Fatalf("original install did not survive: %v", err)
	}
	if man.ID != "hello" {
		t.Errorf("ID = %q, want hello", man.ID)
	}
}

func mustInstalledRoot(t *testing.T) string {
	t.Helper()
	root, err := InstalledRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
