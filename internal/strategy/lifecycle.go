package strategy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Remount mounts again every recorded workspace whose overlay is not mounted — after a
// reboot, or after somebody unmounted one — and says how many it mounted and what it could
// not. A workspace whose directory is gone is left to Sweep; one whose directory holds
// files is never mounted over, which would hide them.
func (maker *OverlayMaker) Remount(ctx context.Context) (int, []error) {
	remounted := 0
	var failed []error
	for _, record := range maker.Records() {
		if ctx.Err() != nil {
			break
		}
		if mountedOverlay(record.Workspace) {
			continue
		}
		entries, err := os.ReadDir(record.Workspace)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", record.Workspace, err))
			continue
		}
		if len(entries) > 0 {
			failed = append(failed, fmt.Errorf("%s holds files while its overlay is not mounted: left as it is", record.Workspace))
			continue
		}
		if _, err := os.Stat(record.Template); err != nil {
			failed = append(failed, fmt.Errorf("%s: its template is gone: %w", record.Workspace, err))
			continue
		}
		layer := maker.layer(record.Workspace)
		if err := maker.Mounter.MountOverlay(record.Lowers(), filepath.Join(layer, "upper"), filepath.Join(layer, "work"), record.Workspace); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", record.Workspace, err))
			continue
		}
		remounted++
	}
	return remounted, failed
}

// Sweep removes the layers no workspace has any more: those half built, with no record, and
// those whose workspace directory is gone. It returns the records it removed, so that their
// templates can be let go. A layer whose workspace is merely unmounted is kept for Remount.
func (maker *OverlayMaker) Sweep() []Record {
	entries, err := os.ReadDir(maker.Layers)
	if err != nil {
		return nil
	}
	var removed []Record
	for _, entry := range entries {
		dir := filepath.Join(maker.Layers, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "workspace"))
		if err != nil {
			if os.RemoveAll(dir) == nil {
				slog.Warn("half-built layer removed", "layer", dir)
			}
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 2 {
			continue
		}
		record := Record{Workspace: lines[0], Template: lines[1]}
		if _, err := os.Lstat(record.Workspace); errors.Is(err, os.ErrNotExist) {
			if os.RemoveAll(dir) == nil {
				slog.Warn("layer of a workspace that is gone removed", "workspace", record.Workspace)
				removed = append(removed, record)
			}
		}
	}
	return removed
}
