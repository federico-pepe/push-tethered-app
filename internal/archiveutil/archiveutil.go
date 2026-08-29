// Package archiveutil extracts .tar.gz/.tgz archives to a local directory,
// shared by internal/catalog (remote downloads) and
// internal/host/procmod (local archive installs) so the extraction and
// zip-slip guard logic exists exactly once.
package archiveutil

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractTarGz extracts the tar.gz archive at src into a fresh temporary
// directory and returns its path. The caller is responsible for removing it
// once done (os.RemoveAll).
//
// Every entry path is checked against a path-traversal ("zip-slip") attack:
// an archive containing ".." segments or an absolute path could otherwise
// write outside the destination directory.
func ExtractTarGz(src string) (dir string, err error) {
	f, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("extracting archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("extracting archive: %w", err)
	}
	defer gz.Close()

	dir, err = os.MkdirTemp("", "push-tethered-app-archive-*")
	if err != nil {
		return "", fmt.Errorf("extracting archive: %w", err)
	}

	if err := extractTar(tar.NewReader(gz), dir); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("extracting archive: %w", err)
	}
	return dir, nil
}

func extractTar(tr *tar.Reader, dir string) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		target, err := safeJoin(dir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// Symlinks, devices, etc: a process module has no legitimate use
			// for them, and following one is exactly the kind of thing a
			// hostile tarball would try. Skip silently.
		}
	}
}

// ResolveWrappedDir returns dir unchanged if it already contains the named
// marker file (e.g. "manifest.json"), or descends into dir's single
// subdirectory and returns that instead if dir has exactly one entry and
// it's a directory. GitHub-style release tarballs commonly wrap their
// contents in a single "<repo>-<tag>/" directory, so a naive extraction
// would otherwise fail to find the marker file at the extracted root.
func ResolveWrappedDir(dir, marker string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
		return dir, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(dir, entries[0].Name()), nil
	}
	return dir, nil
}

// safeJoin resolves name against dir, refusing any path that would escape
// dir — an absolute path, or a "../" that climbs out of it.
func safeJoin(dir, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("archive entry has an absolute path: %q", name)
	}
	target := filepath.Join(dir, name)
	if target != dir && !strings.HasPrefix(target, dir+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry escapes destination: %q", name)
	}
	return target, nil
}
