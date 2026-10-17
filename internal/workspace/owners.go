package workspace

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// claim notes that the workspace at path is one of the repository's own, as well as one
// that borrows from it.
func (workspaces *Workspaces) claim(ctx context.Context, path string, repository repo.Repo) error {
	return workspaces.pools.Own(ctx, repository, path)
}

// RecordOwners writes, once, the list of own workspaces of every pool made before gitd kept
// one: each workspace that borrows from a pool and names its repository is taken as one of
// its own. It runs at start, before any call can change a workspace's configuration.
func (workspaces *Workspaces) RecordOwners(ctx context.Context) {
	for _, dir := range workspaces.pools.All() {
		if pool.HasOwners(dir) {
			continue
		}
		var owned []string
		for _, path := range pool.Workspaces(dir) {
			address, err := workspaces.runner.Config(ctx, filepath.Join(path, ".git", "config"), "gitd.url")
			if repository, ok := repo.Of(address); err == nil && ok && workspaces.pools.Path(repository) == dir {
				owned = append(owned, path)
			}
		}
		if err := pool.RecordOwners(dir, owned); err != nil {
			slog.Warn("the own workspaces of a pool could not be recorded", "pool", dir, "error", err)
		}
	}
}
