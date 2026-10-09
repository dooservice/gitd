package workspace

import (
	"context"
	"path/filepath"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// settle checks the job's revision out, and its submodules when it asks for them. What
// comes back besides is every repository whose pool the workspace now borrows from.
func (workspaces *Workspaces) settle(ctx context.Context, work job) (Result, []repo.Repo, error) {
	result, err := workspaces.checkout(ctx, work)
	if err != nil || !work.submodules {
		return result, []repo.Repo{work.repository}, err
	}
	pending, used, err := workspaces.submodules(ctx, work, "")
	result.Pending = pending
	return result, append(used, work.repository), err
}

// note records that the workspace at path borrows from the pools of used.
func (workspaces *Workspaces) note(ctx context.Context, path string, used []repo.Repo) error {
	for _, repository := range used {
		if err := workspaces.pools.Use(ctx, repository, path); err != nil {
			return err
		}
	}
	return nil
}

// bringUp borrows the pool again, fetches its branches into the workspace — on this disk,
// where nothing has to travel because every object is already the pool's — and settles
// the workspace at the job's revision.
func (workspaces *Workspaces) bringUp(ctx context.Context, work job) (Result, []repo.Repo, error) {
	if err := workspaces.borrow(filepath.Join(work.path, ".git"), work.repository); err != nil {
		return Result{}, nil, err
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: work.path, Local: true, Progress: reporter(work.report, work.repository),
		Args: []string{
			"fetch", "--no-tags", "--progress", "--end-of-options", workspaces.pools.Path(work.repository),
			"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*",
		},
	}); err != nil {
		return Result{}, nil, git.About(err, work.repository.HTTPS())
	}
	return workspaces.settle(ctx, work)
}
