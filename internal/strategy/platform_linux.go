//go:build linux

package strategy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// preferReflink is whether, without an overlay, gitd tries to clone templates: on Linux
// cloning saves disk, not time, so it is used only when configured.
const preferReflink = false

// MountOverlay mounts target as the overlay of upper on lowers, with work as its scratch.
func (Kernel) MountOverlay(lowers []string, upper, work, target string) error {
	options, err := overlayOptions(lowers, upper, work)
	if err != nil {
		return err
	}
	return unix.Mount("overlay", target, "overlay", unix.MS_NOSUID|unix.MS_NODEV, options)
}

// Unmount unmounts target; one already unmounted is not an error.
func (Kernel) Unmount(target string) error {
	if err := unix.Unmount(target, 0); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

// mountedOverlay says whether path is the root of an overlay mount. It needs no privilege.
func mountedOverlay(path string) bool {
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS, &stat); err != nil {
		return false
	}
	if stat.Attributes&unix.STATX_ATTR_MOUNT_ROOT == 0 {
		return false
	}
	var filesystem unix.Statfs_t
	return unix.Statfs(path, &filesystem) == nil && filesystem.Type == unix.OVERLAYFS_SUPER_MAGIC
}

func onOverlay(path string) bool {
	var stats unix.Statfs_t
	return unix.Statfs(path, &stats) == nil && stats.Type == unix.OVERLAYFS_SUPER_MAGIC
}

// clone makes target share source's blocks, with source's mode and modification time.
func clone(source, target string, info fs.FileInfo) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		_ = out.Close()
		if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTTY) {
			return fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(target, info.ModTime(), info.ModTime())
}

// cloneTree clones a whole template at once where the system can; on Linux every file is
// cloned on its own.
func cloneTree(string, string) (bool, error) {
	return false, nil
}
