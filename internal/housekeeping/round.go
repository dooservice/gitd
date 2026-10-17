package housekeeping

import (
	"context"
	"path/filepath"

	"github.com/dooservice/gitd/internal/repo"
)

// round retires the pools nobody borrows from and maintains the rest, adding up what they
// take for Totals.
func (keeper *Keeper) round(ctx context.Context) {
	var count, bytes int64
	defer func() {
		keeper.poolCount.Store(count)
		keeper.poolBytes.Store(bytes)
	}()
	for _, dir := range keeper.pools.All() {
		repository, ok := keeper.repositoryOf(dir)
		if ctx.Err() != nil || !ok {
			continue
		}
		release, err := keeper.pools.Lock(ctx, repository)
		if err != nil {
			// Held by a long sync: not maintained this round, but still there.
			count++
			bytes += size(filepath.Join(dir, "objects"))
			continue
		}
		if keeper.retire(dir) {
			keeper.metrics.PoolBytes.DeleteLabelValues(repository.HTTPS())
		} else {
			count++
			bytes += keeper.maintain(ctx, dir, repository)
		}
		release()
	}
}

// repositoryOf reads the repository back from where its pool is.
func (keeper *Keeper) repositoryOf(dir string) (repo.Repo, bool) {
	relative, err := filepath.Rel(keeper.pools.Dir, dir)
	if err != nil {
		return repo.Repo{}, false
	}
	repository, ok := repo.Of("https://" + filepath.ToSlash(relative))
	// Only a pool that sits exactly where its repository's goes is that repository's.
	return repository, ok && repository.Dir(keeper.pools.Dir) == dir
}
