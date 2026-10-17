package strategy

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

// Mounter mounts and unmounts overlays. gitd has no privilege to; a mounter is whoever
// has: the kernel when gitd runs with CAP_SYS_ADMIN (tests, a container given it), and the
// gitd-mount helper otherwise. lowers are the read-only layers, the topmost first: a
// template, or a warm layer above its template.
type Mounter interface {
	MountOverlay(lowers []string, upper, work, target string) error
	Unmount(target string) error
}

// Kernel mounts with the mount system call, which takes CAP_SYS_ADMIN.
type Kernel struct{}

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

// overlayOptions writes the mount options, refusing a path the kernel would read wrongly:
// commas and colons separate options and layers, and overlayfs has no escape for them.
func overlayOptions(lowers []string, upper, work string) (string, error) {
	if len(lowers) == 0 {
		return "", fmt.Errorf("%w: an overlay needs a lower layer", ErrUnsupported)
	}
	for _, path := range append([]string{upper, work}, lowers...) {
		if strings.ContainsAny(path, ",:\\") {
			return "", fmt.Errorf("%w: %q has a comma, a colon or a backslash, which an overlay cannot name", ErrUnsupported, path)
		}
	}
	return "lowerdir=" + strings.Join(lowers, ":") + ",upperdir=" + upper + ",workdir=" + work, nil
}
