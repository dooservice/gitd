package housekeeping

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/pool"
)

// Sweep removes what a gitd that died left half built: the temporary directories and
// files it builds pools and workspaces in before moving them into place.
func (keeper *Keeper) Sweep() {
	for _, top := range append([]string{keeper.pools.Dir}, keeper.roots...) {
		keeper.sweep(top, 0)
	}
}

func (keeper *Keeper) sweep(dir string, depth int) {
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
		if entry.IsDir() && depth < keeper.sweepDepth && !strings.HasSuffix(entry.Name(), ".git") {
			keeper.sweep(path, depth+1)
		}
	}
}
