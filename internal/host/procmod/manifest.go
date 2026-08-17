package procmod

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// Manifest is a process-loaded module's manifest.json.
//
// Exec is the command line to run, e.g. "python3 run.py" or "node run.js" —
// space-separated, no quoting support (matches this protocol's "small and
// controlled" scope; a module needing an argument with a space in it is
// better served by a wrapper script). Any token that names a file that
// actually exists in the manifest's own directory is resolved to an absolute
// path before spawning, so "run.py" finds the module's own script regardless
// of the host process's current directory, while a bare command name like
// "python3" is left for PATH lookup.
type Manifest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      string `json:"version,omitempty"`
	Author       string `json:"author,omitempty"`
	NeedsMIDIOut bool   `json:"needs_midi_out,omitempty"`
	Exec         string `json:"exec"`
}

// Meta converts the manifest into the same type in-tree modules use.
func (m Manifest) Meta() module.Meta {
	return module.Meta{
		ID:           m.ID,
		Name:         m.Name,
		Author:       m.Author,
		Version:      m.Version,
		NeedsMIDIOut: m.NeedsMIDIOut,
	}
}

// LoadManifest reads and validates manifest.json in dir.
func LoadManifest(dir string) (Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("reading manifest.json: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing manifest.json: %w", err)
	}
	if m.ID == "" {
		return Manifest{}, fmt.Errorf("manifest.json: %q is required", "id")
	}
	if m.Name == "" {
		return Manifest{}, fmt.Errorf("manifest.json: %q is required", "name")
	}
	if m.Exec == "" {
		return Manifest{}, fmt.Errorf("manifest.json: %q is required", "exec")
	}
	return m, nil
}

// resolveExec splits Exec on whitespace and resolves any token that names a
// real file in dir to an absolute path.
//
// This is a heuristic, not a general path resolver, by design: it exists so
// "python3 run.py" finds the module's own run.py wherever the host process
// happens to be running from, without requiring manifest authors to write
// "python3 ./run.py" or the protocol to support quoting and flags properly.
func resolveExec(dir, exec string) ([]string, error) {
	fields := strings.Fields(exec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("exec is empty")
	}
	out := make([]string, len(fields))
	for i, f := range fields {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			out[i] = filepath.Join(dir, f)
		} else {
			out[i] = f
		}
	}
	return out, nil
}

// CopyDir recursively copies src into dst, creating dst if needed and
// preserving file modes — a module's script needs its executable bit intact,
// which a naive copy that recreates files with default permissions would lose.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, in)
		return err
	})
}

// userConfigDir is a seam over os.UserConfigDir so tests can point it at a
// temp directory. A separate copy from internal/host/store.go's own seam
// rather than a shared helper: this package must not import internal/host,
// which itself will import procmod to build installed modules — sharing
// would create a cycle for ten lines of code.
var userConfigDir = os.UserConfigDir

// installedRootOverride, when non-empty, makes InstalledRoot return it
// directly instead of resolving under the real OS config directory. Set only
// by SetInstalledRootForTest.
var installedRootOverride string

// SetInstalledRootForTest points InstalledRoot directly at dir, for tests in
// other packages (internal/host's own tests exercise Runtime.Install/
// Uninstall, which call InstalledRoot indirectly). dir is exactly what
// InstalledRoot will return — unlike overriding userConfigDir, there is no
// "push-tethered-app/installed-modules" joined onto it, so test fixtures can
// be written straight into dir. Returns a function that restores the
// previous behaviour; call it via defer.
func SetInstalledRootForTest(dir string) (restore func()) {
	prev := installedRootOverride
	installedRootOverride = dir
	return func() { installedRootOverride = prev }
}

// Install validates the manifest in srcDir and copies it into this app's
// installed-modules directory, keyed by the manifest's own ID. Pure
// filesystem operation — no hardware, no Runtime, so a CLI flag can install a
// module with no Push connected. Refuses if that ID already has files
// installed (uninstall first to replace it). The caller is responsible for
// checking the ID against currently-running modules if that matters to it —
// this function only knows about disk.
func Install(srcDir string) (Manifest, error) {
	man, err := LoadManifest(srcDir)
	if err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	root, err := InstalledRoot()
	if err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	destDir := filepath.Join(root, man.ID)
	if _, err := os.Stat(destDir); err == nil {
		return Manifest{}, fmt.Errorf("install: %q already has files at %s "+
			"(uninstall it first if you mean to replace it)", man.ID, destDir)
	}
	if err := CopyDir(srcDir, destDir); err != nil {
		return Manifest{}, fmt.Errorf("install: copying files: %w", err)
	}
	return man, nil
}

// Uninstall removes an installed module's files by ID. Pure filesystem
// operation, same reasoning as Install — a CLI flag needs this with no live
// Runtime and no hardware.
func Uninstall(id string) error {
	root, err := InstalledRoot()
	if err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	dir := filepath.Join(root, id)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("uninstall: no installed module %q", id)
	}
	return os.RemoveAll(dir)
}

// ListInstalled returns the manifest of every installed module, without
// spawning anything — for a CLI's -list flag, which should work with no
// hardware and no running Runtime.
func ListInstalled() ([]Manifest, error) {
	root, err := InstalledRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		man, err := LoadManifest(filepath.Join(root, e.Name()))
		if err != nil {
			continue // same "skip, don't fail" stance as Runtime.LoadInstalled
		}
		out = append(out, man)
	}
	return out, nil
}

// InstalledRoot is the directory process-loaded modules are copied into on
// Install, and scanned from at startup.
func InstalledRoot() (string, error) {
	dir := installedRootOverride
	if dir == "" {
		base, err := userConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "push-tethered-app", "installed-modules")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
