package strategy

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// Detect picks how workspaces are made on this machine, from what is wanted — Overlay or
// Reflink by name, Copy, or "" for the best there is — and says why. A nil Maker is Copy:
// the workspace package writes the files, as it always has. Nothing fails here: a strategy
// that cannot run falls back to copy, with the reason.
//
// state is where templates and layers live; roots are where workspaces are made.
func Detect(want Name, state string, roots []string, mounter Mounter) (Maker, string) {
	switch want {
	case Copy:
		return nil, "configured"
	case Reflink:
		if err := probeReflink(state, roots); err != nil {
			return nil, "reflink cannot run here: " + err.Error()
		}
		return ReflinkMaker{}, "configured"
	}
	if err := probeOverlay(state, mounter); err != nil {
		return nil, "overlay cannot run here: " + err.Error()
	}
	return &OverlayMaker{Layers: filepath.Join(state, "layers"), Mounter: mounter}, "an overlay mounts here"
}

// probeOverlay mounts a small overlay under state, writes through it and reads it back.
func probeOverlay(state string, mounter Mounter) error {
	if onOverlay(state) {
		return fmt.Errorf("%s is itself on an overlay, as a container's own filesystem is: put it on a volume or a host directory", state)
	}
	probe, err := os.MkdirTemp(state, "overlay-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(probe)
	lower, upper, work, merged := filepath.Join(probe, "lower"), filepath.Join(probe, "upper"), filepath.Join(probe, "work"), filepath.Join(probe, "merged")
	for _, dir := range []string{lower, upper, work, merged} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(lower, "below"), []byte("below"), 0o644); err != nil {
		return err
	}
	if err := mounter.MountOverlay(lower, upper, work, merged); err != nil {
		return err
	}
	defer func() { _ = mounter.Unmount(merged) }()
	if read, err := os.ReadFile(filepath.Join(merged, "below")); err != nil || string(read) != "below" {
		return fmt.Errorf("the template does not show through: %v", err)
	}
	if err := os.WriteFile(filepath.Join(merged, "above"), []byte("above"), 0o644); err != nil {
		return fmt.Errorf("the layer cannot be written: %w", err)
	}
	if _, err := os.Stat(filepath.Join(lower, "above")); err == nil {
		return fmt.Errorf("a write reached the template")
	}
	return nil
}

// probeReflink clones a file under state, and wants every root on state's filesystem:
// blocks are shared only within one.
func probeReflink(state string, roots []string) error {
	probe, err := os.MkdirTemp(state, "reflink-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(probe)
	source := filepath.Join(probe, "source")
	if err := os.WriteFile(source, []byte("blocks"), 0o644); err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if err := clone(source, filepath.Join(probe, "clone"), info); err != nil {
		return err
	}
	for _, root := range roots {
		if !sameFilesystem(state, root) {
			return fmt.Errorf("%s is not on %s's filesystem", root, state)
		}
	}
	return nil
}

func onOverlay(path string) bool {
	var stats unix.Statfs_t
	return unix.Statfs(path, &stats) == nil && stats.Type == unix.OVERLAYFS_SUPER_MAGIC
}

func sameFilesystem(one, other string) bool {
	first, err := os.Stat(one)
	if err != nil {
		return false
	}
	second, err := os.Stat(other)
	if err != nil {
		return false
	}
	a, okA := first.Sys().(*syscall.Stat_t)
	b, okB := second.Sys().(*syscall.Stat_t)
	return okA && okB && a.Dev == b.Dev
}
