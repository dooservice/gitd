package strategy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// OverlayMaker makes a workspace by mounting, on the workspace's path, a layer of its own
// on top of the template. The layer — its upper and work directories — lives under Layers,
// with a record of which workspace and template it is, so that a restarted gitd can mount
// it again.
type OverlayMaker struct {
	Layers  string
	Mounter Mounter
}

// Name is Overlay.
func (maker *OverlayMaker) Name() Name { return Overlay }

// layer is where the layer of the workspace at dest lives: one directory per workspace
// path, named by its hash.
func (maker *OverlayMaker) layer(dest string) string {
	sum := sha256.Sum256([]byte(dest))
	return filepath.Join(maker.Layers, hex.EncodeToString(sum[:16]))
}

// Make mounts dest as the template with an empty layer of its own on top.
func (maker *OverlayMaker) Make(_ context.Context, template, dest string) error {
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	// The layers are reached only by gitd and gitd-mount: what a sandbox writes into its
	// workspace lands in its upper directory, and no other user may reach it there.
	if err := os.MkdirAll(maker.Layers, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(maker.Layers, 0o700); err != nil {
		return err
	}
	layer := maker.layer(dest)
	if err := os.Mkdir(layer, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	upper, work := filepath.Join(layer, "upper"), filepath.Join(layer, "work")
	for _, dir := range []string{upper, work, dest} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			maker.discard(dest, layer)
			return err
		}
	}
	record := dest + "\n" + template + "\n"
	if err := os.WriteFile(filepath.Join(layer, "workspace"), []byte(record), 0o644); err != nil {
		maker.discard(dest, layer)
		return err
	}
	if err := maker.Mounter.MountOverlay(template, upper, work, dest); err != nil {
		maker.discard(dest, layer)
		return err
	}
	return nil
}

// Unmake unmounts dest and removes its layer.
func (maker *OverlayMaker) Unmake(_ context.Context, dest string) error {
	if err := maker.Mounter.Unmount(dest); err != nil {
		return err
	}
	maker.discard(dest, maker.layer(dest))
	return nil
}

func (maker *OverlayMaker) discard(dest, layer string) {
	_ = os.RemoveAll(layer)
	_ = os.Remove(dest)
}

// LayersOf are the template and the layer of the overlay workspace at dest, when it is one.
func (maker *OverlayMaker) LayersOf(dest string) (lower, upper, work string, ok bool) {
	for _, record := range maker.Records() {
		if record.Workspace == dest {
			layer := maker.layer(dest)
			return record.Template, filepath.Join(layer, "upper"), filepath.Join(layer, "work"), true
		}
	}
	return "", "", "", false
}

// Record is one overlay workspace as its layer records it.
type Record struct {
	Workspace, Template string
}

// Records are the overlay workspaces whose layers are on disk, for mounting them again
// after a restart.
func (maker *OverlayMaker) Records() []Record {
	entries, err := os.ReadDir(maker.Layers)
	if err != nil {
		return nil
	}
	var records []Record
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(maker.Layers, entry.Name(), "workspace"))
		if errors.Is(err, fs.ErrNotExist) || err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 2 {
			records = append(records, Record{Workspace: lines[0], Template: lines[1]})
		}
	}
	return records
}
