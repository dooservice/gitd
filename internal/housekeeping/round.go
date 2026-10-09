package housekeeping

import (
	"context"
	"path/filepath"
	"strings"

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
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 3 {
		return repo.Repo{}, false
	}
	return repo.Of("https://" + strings.Join(parts, "/"))
}
