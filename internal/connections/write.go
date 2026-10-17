package connections

import (
	"os"
	"path/filepath"
)

// writeAtomically writes data to path so that a crash leaves the old file or the new one,
// never half of either: written aside, synced, moved into place, and the directory synced.
// The file is readable by gitd's user only.
func writeAtomically(path string, data []byte) error {
	directory := filepath.Dir(path)
	aside, err := os.CreateTemp(directory, ".writing-*")
	if err != nil {
		return err
	}
	defer os.Remove(aside.Name())
	if err := aside.Chmod(0o600); err != nil {
		_ = aside.Close()
		return err
	}
	if _, err := aside.Write(data); err != nil {
		_ = aside.Close()
		return err
	}
	if err := aside.Sync(); err != nil {
		_ = aside.Close()
		return err
	}
	if err := aside.Close(); err != nil {
		return err
	}
	if err := os.Rename(aside.Name(), path); err != nil {
		return err
	}
	opened, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer opened.Close()
	return opened.Sync()
}
