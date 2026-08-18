package procmod

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadManifestValid(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{"id":"hello","name":"Hello","exec":"python3 run.py","needs_midi_out":true}`)

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.ID != "hello" || m.Name != "Hello" || m.Exec != "python3 run.py" || !m.NeedsMIDIOut {
		t.Errorf("parsed = %+v", m)
	}
	meta := m.Meta()
	if meta.ID != "hello" || !meta.NeedsMIDIOut {
		t.Errorf("Meta() = %+v", meta)
	}
}

func TestLoadManifestRequiredFields(t *testing.T) {
	tests := []struct {
		name, json string
	}{
		{"missing id", `{"name":"x","exec":"y"}`},
		{"missing name", `{"id":"x","exec":"y"}`},
		{"missing exec", `{"id":"x","name":"y"}`},
		{"empty id", `{"id":"","name":"x","exec":"y"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir, tt.json)
			if _, err := LoadManifest(dir); err == nil {
				t.Error("want an error, got nil")
			}
		})
	}
}

func TestLoadManifestMissingFile(t *testing.T) {
	if _, err := LoadManifest(t.TempDir()); err == nil {
		t.Error("want an error when manifest.json does not exist")
	}
}

func TestLoadManifestMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{not valid json`)
	if _, err := LoadManifest(dir); err == nil {
		t.Error("want an error for malformed JSON")
	}
}

// TestResolveExecJoinsFilesThatExist pins the heuristic: a token naming a real
// file in the module's own directory becomes an absolute path (so the script
// is found regardless of the host's own working directory); a bare command
// name is left alone for PATH lookup.
func TestResolveExecJoinsFilesThatExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.py"), []byte("#!/usr/bin/env python3\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	fields, err := resolveExec(dir, "python3 run.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields = %v, want 2", fields)
	}
	if fields[0] != "python3" {
		t.Errorf("fields[0] = %q, want unresolved %q (not a file in dir)", fields[0], "python3")
	}
	want := filepath.Join(dir, "run.py")
	if fields[1] != want {
		t.Errorf("fields[1] = %q, want %q", fields[1], want)
	}
}

func TestResolveExecEmpty(t *testing.T) {
	if _, err := resolveExec(t.TempDir(), ""); err == nil {
		t.Error("want an error for an empty exec string")
	}
	if _, err := resolveExec(t.TempDir(), "   "); err == nil {
		t.Error("want an error for a whitespace-only exec string")
	}
}

func TestCopyDirPreservesExecutableBit(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "run.py"), []byte("#!/usr/bin/env python3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(src, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "assets", "icon.png"), []byte("fake png"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "copied")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	info, err := os.Stat(filepath.Join(dst, "run.py"))
	if err != nil {
		t.Fatalf("copied run.py missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("copied run.py mode = %v, executable bit was lost", info.Mode())
	}

	b, err := os.ReadFile(filepath.Join(dst, "assets", "icon.png"))
	if err != nil {
		t.Fatalf("nested file missing: %v", err)
	}
	if string(b) != "fake png" {
		t.Errorf("nested file content = %q", b)
	}
}

func TestInstalledRootCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	prev := userConfigDir
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = prev })

	dir, err := InstalledRoot()
	if err != nil {
		t.Fatalf("InstalledRoot: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("InstalledRoot() = %q, does not exist as a directory: %v", dir, err)
	}
}
