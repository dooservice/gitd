//go:build !windows

package files

import "os"

func keepPrivate(file *os.File) error {
	return file.Chmod(0o600)
}

// syncDirectory makes a rename in directory survive a crash.
func syncDirectory(directory string) error {
	opened, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer opened.Close()
	return opened.Sync()
}
