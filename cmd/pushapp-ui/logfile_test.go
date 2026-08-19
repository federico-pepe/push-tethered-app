package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = prev })
	return dir
}

func TestOpenLogFileCreatesDirAndFile(t *testing.T) {
	base := withTempConfigDir(t)

	path, f, err := openLogFile()
	if err != nil {
		t.Fatalf("openLogFile: %v", err)
	}
	defer f.Close()

	want := filepath.Join(base, "push-tethered-app", "logs", "pushapp-ui.log")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file does not exist: %v", err)
	}
}

func TestOpenLogFileTruncatesOnEachRun(t *testing.T) {
	withTempConfigDir(t)

	_, f1, err := openLogFile()
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := f1.WriteString("first run\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f1.Close()

	path, f2, err := openLogFile()
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer f2.Close()

	// This is what makes it a "last run" log rather than an ever-growing
	// one: opening it again must not see the previous run's content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("log file was not truncated on reopen, got %q", data)
	}
}

func TestOpenLogFilePropagatesUserConfigDirError(t *testing.T) {
	prev := userConfigDir
	sentinel := errors.New("no config dir")
	userConfigDir = func() (string, error) { return "", sentinel }
	t.Cleanup(func() { userConfigDir = prev })

	if _, _, err := openLogFile(); !errors.Is(err, sentinel) {
		t.Errorf("openLogFile error = %v, want %v", err, sentinel)
	}
}
