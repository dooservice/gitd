//go:build linux

package mounthelper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// errRefused is a request outside what the configuration allows.
var errRefused = errors.New("refused")

// pinned is a directory opened once, by descriptor, so that what is checked is what is
// used: its Path names the descriptor itself, never the name it was opened by.
type pinned struct {
	fd   int
	Path string
}

func (dir pinned) Close() { _ = unix.Close(dir.fd) }

// pin opens path, a directory beneath base, without following any symbolic link on the way
// — base included, from / — and without leaving base, and checks it is owned by uid.
func pin(base, path string, uid int) (pinned, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return pinned{}, fmt.Errorf("%w: %q is not a clean absolute path", errRefused, path)
	}
	relative, err := filepath.Rel(base, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return pinned{}, fmt.Errorf("%w: %s is not beneath %s", errRefused, path, base)
	}
	baseFD, err := unix.Openat2(unix.AT_FDCWD, base, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return pinned{}, fmt.Errorf("%w: %s: %v", errRefused, base, err)
	}
	defer unix.Close(baseFD)
	fd, err := unix.Openat2(baseFD, relative, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return pinned{}, fmt.Errorf("%w: %s: %v", errRefused, path, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || int(stat.Uid) != uid {
		_ = unix.Close(fd)
		return pinned{}, fmt.Errorf("%w: %s is not the caller's", errRefused, path)
	}
	return pinned{fd: fd, Path: fmt.Sprintf("/proc/self/fd/%d", fd)}, nil
}

// pinUnderRoots pins path beneath whichever root holds it.
func pinUnderRoots(roots []string, path string, uid int) (pinned, error) {
	return pinUnderAny(roots, "roots", path, uid)
}

// pinUnderAny pins path beneath whichever of bases it lies under: one tenant's templates or
// layers among every tenant's.
func pinUnderAny(bases []string, what, path string, uid int) (pinned, error) {
	for _, base := range bases {
		if path != base && strings.HasPrefix(path, base+"/") {
			return pin(base, path, uid)
		}
	}
	return pinned{}, fmt.Errorf("%w: %s is in none of the %s", errRefused, path, what)
}

// empty says whether the pinned directory has nothing in it.
func empty(dir pinned) bool {
	entries, err := os.ReadDir(dir.Path)
	return err == nil && len(entries) == 0
}
