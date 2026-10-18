//go:build linux

package mounthelper

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

// Do carries a request out for the caller uid, within config, or says why not.
func Do(config Config, uid int, request Request) error {
	if uid != config.UID {
		return fmt.Errorf("%w: uid %d may not ask", errRefused, uid)
	}
	switch request.Op {
	case "mount":
		return mountOverlay(config, uid, request)
	case "unmount":
		return unmountOverlay(config, uid, request.Target)
	}
	return fmt.Errorf("%w: no operation %q", errRefused, request.Op)
}

func mountOverlay(config Config, uid int, request Request) error {
	layers, err := checkMount(config, uid, request)
	if err != nil {
		return err
	}
	defer layers.close()
	lowers := make([]string, len(layers.lowers))
	for index, lower := range layers.lowers {
		lowers[index] = lower.Path
	}
	options := "lowerdir=" + strings.Join(lowers, ":") + ",upperdir=" + layers.upper.Path + ",workdir=" + layers.work.Path
	return unix.Mount("overlay", layers.target.Path, "overlay", unix.MS_NOSUID|unix.MS_NODEV, options)
}

// maxLowers is how many read-only layers a workspace may stand on: its template, and a warm
// layer above it.
const maxLowers = 2

// mountLayers are a mount request's directories, pinned and checked.
type mountLayers struct {
	lowers              []pinned
	upper, work, target pinned
}

func (layers mountLayers) close() {
	for _, dir := range append([]pinned{layers.upper, layers.work, layers.target}, layers.lowers...) {
		if dir.Path != "" {
			dir.Close()
		}
	}
}

// checkMount pins a mount request's directories, each where it belongs — the lower layers
// among the templates, upper and work among the layers, the target in a root and empty —
// all the caller's, reached without a symbolic link.
func checkMount(config Config, uid int, request Request) (mountLayers, error) {
	var layers mountLayers
	var err error
	lowers := strings.Split(request.Lower, ":")
	if len(lowers) > maxLowers {
		return mountLayers{}, fmt.Errorf("%w: at most %d lower layers", errRefused, maxLowers)
	}
	for _, lower := range lowers {
		pinned, err := pinUnderAny(config.Templates, "templates", lower, uid)
		if err != nil {
			layers.close()
			return mountLayers{}, err
		}
		layers.lowers = append(layers.lowers, pinned)
	}
	if layers.upper, err = pinUnderAny(config.Layers, "layers", request.Upper, uid); err != nil {
		layers.close()
		return mountLayers{}, err
	}
	if layers.work, err = pinUnderAny(config.Layers, "layers", request.Work, uid); err != nil {
		layers.close()
		return mountLayers{}, err
	}
	if layers.target, err = pinUnderRoots(config.Roots, request.Target, uid); err != nil {
		layers.close()
		return mountLayers{}, err
	}
	if !empty(layers.target) {
		layers.close()
		return mountLayers{}, fmt.Errorf("%w: %s is not empty", errRefused, request.Target)
	}
	return layers, nil
}

// unmountOverlay unmounts target only if it is the root of an overlay mounted beneath a
// root: never any other mount. Checked and unmounted through one descriptor, so nothing
// swapped in between can redirect it.
func unmountOverlay(config Config, uid int, target string) error {
	pinned, err := pinUnderRoots(config.Roots, target, uid)
	if err != nil {
		return err
	}
	defer pinned.Close()
	var stat unix.Statx_t
	if err := unix.Statx(pinned.fd, "", unix.AT_EMPTY_PATH, unix.STATX_BASIC_STATS, &stat); err != nil {
		return err
	}
	if stat.Attributes_mask&unix.STATX_ATTR_MOUNT_ROOT == 0 {
		return fmt.Errorf("%w: this kernel cannot say what is a mount", errRefused)
	}
	if stat.Attributes&unix.STATX_ATTR_MOUNT_ROOT == 0 {
		return nil // not mounted: nothing to undo
	}
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(pinned.fd, &filesystem); err != nil {
		return err
	}
	if filesystem.Type != unix.OVERLAYFS_SUPER_MAGIC {
		return fmt.Errorf("%w: %s is a mount, but not an overlay", errRefused, target)
	}
	// Detached: the descriptor that made the checks holds the mount, so a plain unmount
	// would find it busy. The mount leaves the tree at once and is gone when that last
	// descriptor closes, and it is still exactly the mount that was checked.
	return unix.Unmount(pinned.Path, unix.MNT_DETACH)
}
