package archiveutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// writeTarGz builds a .tar.gz containing entries and returns its path.
func writeTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGzWritesFiles(t *testing.T) {
	archive := writeTarGz(t, map[string]string{
		"manifest.json":  `{"id":"hello"}`,
		"assets/foo.txt": "hi",
	})

	dir, err := ExtractTarGz(archive)
	if err != nil {
		t.Fatalf("ExtractTarGz: %v", err)
	}
	defer os.RemoveAll(dir)

	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil || string(b) != `{"id":"hello"}` {
		t.Errorf("manifest.json = %q, %v", b, err)
	}
	b, err = os.ReadFile(filepath.Join(dir, "assets", "foo.txt"))
	if err != nil || string(b) != "hi" {
		t.Errorf("assets/foo.txt = %q, %v", b, err)
	}
}

func TestExtractTarGzRejectsPathTraversal(t *testing.T) {
	archive := writeTarGz(t, map[string]string{
		"../evil.txt": "gotcha",
	})

	if dir, err := ExtractTarGz(archive); err == nil {
		os.RemoveAll(dir)
		t.Error("ExtractTarGz did not reject a path-traversal entry")
	}
}

func TestExtractTarGzRejectsAbsolutePath(t *testing.T) {
	archive := writeTarGz(t, map[string]string{
		"/etc/evil.txt": "gotcha",
	})

	if dir, err := ExtractTarGz(archive); err == nil {
		os.RemoveAll(dir)
		t.Error("ExtractTarGz did not reject an absolute path entry")
	}
}

func TestResolveWrappedDirFindsMarkerAtRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveWrappedDir(dir, "manifest.json")
	if err != nil {
		t.Fatalf("ResolveWrappedDir: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestResolveWrappedDirDescendsSingleSubdir(t *testing.T) {
	dir := t.TempDir()
	wrapped := filepath.Join(dir, "repo-v1.0.0")
	if err := os.MkdirAll(wrapped, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrapped, "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveWrappedDir(dir, "manifest.json")
	if err != nil {
		t.Fatalf("ResolveWrappedDir: %v", err)
	}
	if got != wrapped {
		t.Errorf("got %q, want %q", got, wrapped)
	}
}
