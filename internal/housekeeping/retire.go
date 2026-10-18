package housekeeping

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// MinimumUnused is the least a pool must have gone unused before it is retired on request.
const MinimumUnused = 24 * time.Hour

// Retire removes now every pool no workspace borrows from and nothing has used for at least
// unusedFor, and says how many there were and the disk they took. Totals then count the
// pools that are left.
func (keeper *Keeper) Retire(ctx context.Context, unusedFor time.Duration) (int, int64) {
	unusedFor = max(unusedFor, MinimumUnused)
	var retired, kept int
	var freed, keptBytes int64
	for _, dir := range keeper.pools.All() {
		repository, ok := keeper.repositoryOf(dir)
		if !ok {
			continue
		}
		taken := size(filepath.Join(dir, "objects"))
		if ctx.Err() == nil && keeper.retireLocked(ctx, dir, repository, unusedFor) {
			retired++
			freed += taken
			continue
		}
		kept++
		keptBytes += taken
	}
	keeper.poolCount.Store(int64(kept))
	keeper.poolBytes.Store(keptBytes)
	return retired, freed
}

// retireLocked retires the pool at dir under its lock; a pool somebody holds is kept.
func (keeper *Keeper) retireLocked(ctx context.Context, dir string, repository repo.Repo, unusedFor time.Duration) bool {
	release, err := keeper.pools.Lock(ctx, repository)
	if err != nil {
		return false
	}
	defer release()
	if !keeper.retireAfter(dir, unusedFor) {
		return false
	}
	keeper.metrics.PoolBytes.DeleteLabelValues(repository.HTTPS())
	return true
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
// a pool's objects directory. A submodule named `vendor/web` lives at
// `.git/modules/vendor/web`, so the modules are walked rather than globbed.
func borrows(path, objects string) bool {
	// A workspace's own, or a bare sandbox repository's.
	alternates := []string{
		filepath.Join(path, ".git", "objects", "info", "alternates"),
		filepath.Join(path, "objects", "info", "alternates"),
	}
	_ = filepath.WalkDir(filepath.Join(path, ".git", "modules"), func(file string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(file, filepath.Join("objects", "info", "alternates")) {
			alternates = append(alternates, file)
		}
		return nil
	})
	return slices.ContainsFunc(alternates, func(file string) bool {
		data, err := os.ReadFile(file)
		if err != nil {
			return false
		}
		for _, line := range strings.Fields(string(data)) {
			if line == objects || carried(file, line, objects) {
				return true
			}
		}
		return false
	})
}

// carried says whether a relative alternates line — a workspace carrying its pool inside
// itself, as a pod's does — reaches the pool's objects, mounted where it says.
func carried(alternates, line, objects string) bool {
	if filepath.IsAbs(line) {
		return false
	}
	mounted, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(alternates)), line))
	if err != nil {
		return false
	}
	pool, err := os.Stat(objects)
	return err == nil && os.SameFile(mounted, pool)
}
