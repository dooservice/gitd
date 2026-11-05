//go:build !windows

package workspace

import (
	"os"
	"syscall"
)

// isWhiteout says whether a character device is overlay's mark of a deletion.
func isWhiteout(file string) bool {
	info, err := os.Lstat(file)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Rdev == 0
}
