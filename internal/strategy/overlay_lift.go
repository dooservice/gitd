package strategy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Lifted is an overlay workspace's own layer taken off it, unmounted, without the
// workspace's .git, which is the workspace's and not what was installed in it. Upper is
// ready to be moved away; then Drop removes what is left, or Restore, when it was not
// moved after all, puts the workspace back as it was.
type Lifted struct {
	Upper  string
	maker  *OverlayMaker
	record Record
	layer  string
}

// gitAside is where the workspace's .git waits, inside its layer, while its upper
// directory is lifted.
const gitAside = "git-aside"

// ErrHeld is why a layer is not lifted: its workspace is still mounted somewhere else, as a
// sandbox binds it, and what the sandbox writes would land in the layer after it was moved.
var ErrHeld = errors.New("the workspace is still mounted elsewhere, as a sandbox binds it: stop what uses it first")

// Lift unmounts the overlay workspace at dest and sets its .git aside. A workspace still
// mounted elsewhere once unmounted at dest is put back, and ErrHeld answered.
func (maker *OverlayMaker) Lift(dest string) (Lifted, error) {
	record, ok := maker.RecordOf(dest)
	if !ok {
		return Lifted{}, fmt.Errorf("%s is not an overlay workspace", dest)
	}
	layer := maker.layer(dest)
	lifted := Lifted{Upper: filepath.Join(layer, "upper"), maker: maker, record: record, layer: layer}
	dev, mounted := mountedFilesystem(dest)
	if err := maker.Mounter.Unmount(dest); err != nil {
		return Lifted{}, err
	}
	if mounted && heldElsewhere(dev) {
		return Lifted{}, errors.Join(ErrHeld, lifted.Restore())
	}
	aside := filepath.Join(layer, gitAside)
	if err := os.Rename(filepath.Join(lifted.Upper, ".git"), aside); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Lifted{}, errors.Join(err, lifted.Restore())
	}
	return lifted, nil
}

// Restore puts the .git back and mounts the workspace again, as it was before Lift.
func (lifted Lifted) Restore() error {
	aside := filepath.Join(lifted.layer, gitAside)
	if _, err := os.Lstat(aside); err == nil {
		if err := os.Rename(aside, filepath.Join(lifted.Upper, ".git")); err != nil {
			return err
		}
	}
	return lifted.maker.Mounter.MountOverlay(lifted.record.Lowers(), lifted.Upper, filepath.Join(lifted.layer, "work"), lifted.record.Workspace)
}

// Drop removes what is left of the workspace once its upper directory was moved away.
func (lifted Lifted) Drop() {
	lifted.maker.discard(lifted.record.Workspace, lifted.layer)
}
