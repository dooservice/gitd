package workspace

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/repo"
)

// adoptSubmodules takes in every checked out submodule of an adopted clone: its pool is
// seeded from the submodule's own git directory when there is none, the directory is
// linked to it and repacked, and the submodule is pointed at the pool from then on, as a
// workspace gitd made would be.
//
// One that cannot be taken in is left as it was. It still works, it only keeps its copy.
func (workspaces *Workspaces) adoptSubmodules(
	ctx context.Context, path string, parent repo.Repo, report Report,
) ([]repo.Repo, int64, error) {
	declared, err := workspaces.declaredIn(ctx, path)
	if err != nil {
		return nil, 0, err
	}
	var used []repo.Repo
	var released int64
	for _, submodule := range declared {
		gitDir := filepath.Join(path, ".git", "modules", submodule.name)
		workTree := filepath.Join(path, submodule.path)
		repository, named := repo.Of(repo.Relative(submodule.url, parent))
		if _, err := os.Stat(filepath.Join(workTree, ".git")); err != nil || !named {
			continue
		}
		if err := workspaces.pools.Seed(ctx, repository, gitDir, reporter(report, repository)); err != nil {
			continue
		}
		freed, err := workspaces.takeIn(ctx, workTree, gitDir, repository, report)
		if err != nil {
			continue
		}
		poolURL := "file://" + workspaces.pools.Path(repository)
		if err := workspaces.runner.SetConfig(ctx, path, "submodule."+submodule.name+".url", poolURL); err != nil {
			return nil, 0, err
		}
		if err := workspaces.runner.SetConfig(ctx, workTree, "remote.origin.url", poolURL); err != nil {
			return nil, 0, err
		}
		used = append(used, repository)
		released += freed
	}
	return used, released, nil
}
