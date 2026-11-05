//go:build !windows

package strategy

import (
	"os"
	"syscall"
)

// deviceOf is the number of the filesystem path is on.
func deviceOf(path string) (uint64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Dev), true
}
