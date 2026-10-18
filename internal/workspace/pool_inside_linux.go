//go:build linux

package workspace

import (
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/dooservice/gitd/internal/git"
)

// poolStillMounted refuses to remove a workspace whose `.git/gitd-pool` is still a mount:
// removing would reach into the pool.
func poolStillMounted(path string) error {
	mounted := filepath.Join(path, ".git", poolMount)
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, mounted, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS, &stat); err != nil {
		return nil
	}
	if stat.Attributes_mask&unix.STATX_ATTR_MOUNT_ROOT != 0 && stat.Attributes&unix.STATX_ATTR_MOUNT_ROOT != 0 {
		return git.Fail(git.Busy, "%s is still mounted: unmount it before removing the workspace", mounted)
	}
	return nil
}
