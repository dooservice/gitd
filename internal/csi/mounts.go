//go:build linux

package csi

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// bind mounts from onto to, read-only when asked; with recursive, what is mounted inside
// from comes along — the pool inside a workspace, which the pod needs.
func bind(from, to string, readOnly, recursive bool) error {
	flags := uintptr(unix.MS_BIND)
	if recursive {
		flags |= unix.MS_REC
	}
	if err := unix.Mount(from, to, "", flags, ""); err != nil {
		return fmt.Errorf("binding %s onto %s: %w", from, to, err)
	}
	if !readOnly {
		return nil
	}
	if err := unix.Mount("", to, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV, ""); err != nil {
		_ = unix.Unmount(to, unix.MNT_DETACH)
		return fmt.Errorf("making %s read-only: %w", to, err)
	}
	return nil
}

// mounted says whether something is mounted at path.
func mounted(path string) bool {
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS, &stat); err != nil {
		return false
	}
	return stat.Attributes_mask&unix.STATX_ATTR_MOUNT_ROOT != 0 && stat.Attributes&unix.STATX_ATTR_MOUNT_ROOT != 0
}

// unmount unmounts path when something is mounted there; nothing mounted is no error.
func unmount(path string) error {
	if !mounted(path) {
		return nil
	}
	if err := unix.Unmount(path, 0); err != nil && !errors.Is(err, unix.EINVAL) {
		return unix.Unmount(path, unix.MNT_DETACH)
	}
	return nil
}

// directory makes path a directory, if it is not one already.
func directory(path string) error {
	return os.MkdirAll(path, 0o750)
}
