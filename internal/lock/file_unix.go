//go:build !windows

package lock

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes file's exclusive lock without waiting: held is false, with no error,
// when another process has it.
func tryLockFile(file *os.File) (held bool, err error) {
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
