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
func (maker *OverlayMaker) Make(ctx context.Context, template, dest string) error {
	return maker.MakeWarm(ctx, template, "", dest)
}

// MakeWarm mounts dest as the template, a warm layer above it when warm is set, and an
// empty layer of its own on top.
func (maker *OverlayMaker) MakeWarm(_ context.Context, template, warm, dest string) error {
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	// The layers keep the mode gitd-mount expects; a setuid file a sandbox leaves in its upper
	// directory is kept harmless by the nosuid filesystem the threat model asks for.
	layer := maker.layer(dest)
	upper, work := filepath.Join(layer, "upper"), filepath.Join(layer, "work")
	for _, dir := range []string{upper, work, dest} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			maker.discard(dest, layer)
			return err
		}
	}
	record := dest + "\n" + template + "\n"
	if warm != "" {
		record += warm + "\n"
	}
	if err := os.WriteFile(filepath.Join(layer, "workspace"), []byte(record), 0o644); err != nil {
		maker.discard(dest, layer)
		return err
	}
	if err := maker.Mounter.MountOverlay(Record{Template: template, Warm: warm}.Lowers(), upper, work, dest); err != nil {
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

// RecordOf is the record of the overlay workspace at dest, when it is one.
func (maker *OverlayMaker) RecordOf(dest string) (Record, bool) {
	for _, record := range maker.Records() {
		if record.Workspace == dest {
			return record, true
		}
	}
	return Record{}, false
}

// Record is one overlay workspace as its layer records it: its template, and the warm layer
// above it, if any.
type Record struct {
	Workspace, Template, Warm string
}

// Lowers are the record's read-only layers, the topmost first.
func (record Record) Lowers() []string {
	if record.Warm != "" {
		return []string{record.Warm, record.Template}
	}
	return []string{record.Template}
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
		switch len(lines) {
		case 2:
			records = append(records, Record{Workspace: lines[0], Template: lines[1]})
		case 3:
			records = append(records, Record{Workspace: lines[0], Template: lines[1], Warm: lines[2]})
		}
	}
	return records
}
