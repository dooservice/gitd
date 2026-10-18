//go:build darwin

package strategy

import (
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// preferReflink is whether, without an overlay, gitd tries to clone templates: on macOS,
// APFS clones a whole tree at once, almost free in time and disk.
const preferReflink = true

// MountOverlay is unsupported: macOS has no overlay.
func (Kernel) MountOverlay([]string, string, string, string) error {
	return fmt.Errorf("%w: macOS has no overlay", ErrUnsupported)
}

// Unmount has nothing to unmount on macOS.
func (Kernel) Unmount(string) error { return nil }

func mountedOverlay(string) bool { return false }

func onOverlay(string) bool { return false }

func mountedFilesystem(string) (uint64, bool) { return 0, false }

func heldElsewhere(uint64) bool { return false }

// clone makes target an APFS clone of source, sharing its blocks, with source's mode and
// modification time.
func clone(source, target string, info fs.FileInfo) error {
	if err := unix.Clonefile(source, target, unix.CLONE_NOFOLLOW); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return os.Chtimes(target, info.ModTime(), info.ModTime())
}

// cloneTree clones a whole template at once with clonefile(2), which keeps modification
// times: one call, however many files.
func cloneTree(template, dest string) (bool, error) {
	if err := unix.Clonefile(template, dest, unix.CLONE_NOFOLLOW); err != nil {
		return true, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return true, nil
}
