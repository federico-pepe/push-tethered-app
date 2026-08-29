package procmod

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/federico-pepe/push-tethered-app/internal/archiveutil"
)

// isArchive reports whether path names a .tar.gz or .tgz file rather than a
// directory to install as-is.
func isArchive(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

// resolveInstallSource returns a directory to install from srcPath: srcPath
// itself if it's already a directory, or the extracted, unwrapped contents
// of srcPath if it's a .tar.gz/.tgz archive. cleanup removes any temporary
// extraction directory and is always safe to call, even for a plain
// directory srcPath.
func resolveInstallSource(srcPath string) (dir string, cleanup func(), err error) {
	if !isArchive(srcPath) {
		return srcPath, func() {}, nil
	}
	extracted, err := archiveutil.ExtractTarGz(srcPath)
	if err != nil {
		return "", nil, fmt.Errorf("install: %w", err)
	}
	cleanup = func() { os.RemoveAll(extracted) }
	resolved, err := archiveutil.ResolveWrappedDir(extracted, "manifest.json")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("install: %w", err)
	}
	return resolved, cleanup, nil
}

// InstallFromPath installs a module from either a directory or a
// .tar.gz/.tgz archive — the single entry point cmd/pushapp and
// pushapp-ui's folder/file picker should both call, so manual installs and
// catalog-driven installs share one code path.
func InstallFromPath(srcPath string) (Manifest, error) {
	dir, cleanup, err := resolveInstallSource(srcPath)
	if err != nil {
		return Manifest{}, err
	}
	defer cleanup()
	return Install(dir)
}

// Update replaces an already-installed module's files with the contents of
// srcPath (a directory or .tar.gz/.tgz archive), refusing if the new
// manifest's ID doesn't match id. Unlike Install, this does not refuse when
// destDir already exists — that's the whole point of an update. The old
// files are removed only after the new ones extract and validate cleanly,
// so a failed download or a bad archive leaves the existing install intact.
func Update(id, srcPath string) (Manifest, error) {
	dir, cleanup, err := resolveInstallSource(srcPath)
	if err != nil {
		return Manifest{}, err
	}
	defer cleanup()

	man, err := LoadManifest(dir)
	if err != nil {
		return Manifest{}, fmt.Errorf("update: %w", err)
	}
	if man.ID != id {
		return Manifest{}, fmt.Errorf("update: archive contains module %q, expected %q", man.ID, id)
	}

	root, err := InstalledRoot()
	if err != nil {
		return Manifest{}, fmt.Errorf("update: %w", err)
	}
	destDir := filepath.Join(root, id)
	stagingDir := destDir + ".update"
	os.RemoveAll(stagingDir)
	if err := CopyDir(dir, stagingDir); err != nil {
		os.RemoveAll(stagingDir)
		return Manifest{}, fmt.Errorf("update: copying files: %w", err)
	}

	if err := os.RemoveAll(destDir); err != nil {
		os.RemoveAll(stagingDir)
		return Manifest{}, fmt.Errorf("update: removing old files: %w", err)
	}
	if err := os.Rename(stagingDir, destDir); err != nil {
		return Manifest{}, fmt.Errorf("update: replacing old files: %w", err)
	}
	return man, nil
}
