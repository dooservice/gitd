package housekeeping

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// Sweep removes what a gitd that died left half built: the temporary directories and
// files it builds pools and workspaces in before moving them into place.
func (keeper *Keeper) Sweep() {
	// A pool is `<host>/<path>.git`, and a path has at most repo.MaxDepth segments.
	keeper.sweep(keeper.pools.Dir, 0, repo.MaxDepth)
	for _, root := range keeper.roots {
		keeper.sweep(root, 0, keeper.settings.SweepDepth)
	}
	if keeper.templates != nil {
		keeper.templates.Sweep()
	}
	if keeper.overlay != nil && keeper.templates != nil {
		// Layers whose workspace is gone free their templates for retirement.
		for _, record := range keeper.overlay.Sweep() {
			if key, ok := keeper.templates.KeyOf(record.Template); ok {
				_ = keeper.templates.Let(context.Background(), key, record.Workspace)
			}
		}
	}
}

func (keeper *Keeper) sweep(dir string, depth, limit int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if pool.IsTemp(entry.Name()) {
			if os.RemoveAll(path) == nil {
				slog.Warn("half-built copy removed", "path", path)
			}
			continue
		}
		if strings.HasPrefix(entry.Name(), ".gitd-probe-") {
			// Where a crash interrupted the start-up's overlay probe: an empty directory.
			_ = os.Remove(path)
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".git") {
			// A registry or a file written atomically inside a repository, left half written.
			keeper.removeTemps(path)
		} else if depth < limit {
			keeper.sweep(path, depth+1, limit)
		}
	}
}

// removeTemps removes the temporary files Temp named directly inside dir.
func (keeper *Keeper) removeTemps(dir string) {
	temps, _ := filepath.Glob(filepath.Join(dir, "*.gitd-tmp-*"))
	for _, temp := range temps {
		if os.RemoveAll(temp) == nil {
			slog.Warn("half-built copy removed", "path", temp)
		}
	}
}
