package housekeeping

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/pool"
)

// MinimumUnused is the least a pool must have gone unused before it is retired on request.
const MinimumUnused = 24 * time.Hour

// Retire removes now every pool no workspace borrows from and nothing has used for at least
// unusedFor, and says how many there were and the disk they took.
func (keeper *Keeper) Retire(ctx context.Context, unusedFor time.Duration) (int, int64) {
	unusedFor = max(unusedFor, MinimumUnused)
	retired, freed := 0, int64(0)
	for _, dir := range keeper.pools.All() {
		repository, ok := keeper.repositoryOf(dir)
		if ctx.Err() != nil || !ok {
			continue
		}
		release, err := keeper.pools.Lock(ctx, repository)
		if err != nil {
			continue
		}
		taken := size(filepath.Join(dir, "objects"))
		if keeper.retireAfter(dir, unusedFor) {
			keeper.metrics.PoolBytes.DeleteLabelValues(repository.HTTPS())
			retired++
			freed += taken
		}
		release()
	}
	keeper.poolCount.Add(int64(-retired))
	keeper.poolBytes.Add(-freed)
	return retired, freed
}

// retire removes a pool that no workspace has borrowed from for as long as the settings say.
func (keeper *Keeper) retire(dir string) bool {
	return keeper.retireAfter(dir, keeper.settings.RetireAfter)
}

// retireAfter removes a pool that no workspace has borrowed from for unusedFor. A workspace
// counts while its alternates still point at the pool, wherever the registry says it is:
// an environment deleted by hand no longer does.
func (keeper *Keeper) retireAfter(dir string, unusedFor time.Duration) bool {
	objects := filepath.Join(dir, "objects")
	borrowed := slices.ContainsFunc(pool.Workspaces(dir), func(path string) bool {
		return borrows(path, objects)
	})
	if borrowed || time.Since(pool.LastUsed(dir)) < unusedFor {
		return false
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Error("pool not retired", "pool", dir, "error", err)
		return false
	}
	slog.Info("pool retired", "pool", dir)
	return true
}

// borrows is whether the repository at path, or one of its submodules, reads objects from
// a pool's objects directory.
func borrows(path, objects string) bool {
	alternates, _ := filepath.Glob(filepath.Join(path, ".git", "modules", "*", "objects", "info", "alternates"))
	alternates = append(alternates, filepath.Join(path, ".git", "objects", "info", "alternates"))
	return slices.ContainsFunc(alternates, func(file string) bool {
		data, err := os.ReadFile(file)
		return err == nil && slices.Contains(strings.Fields(string(data)), objects)
	})
}
