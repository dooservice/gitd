package workspace

import (
	"context"
	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
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

// settled notes what a workspace just made or moved borrows from, and the commit it is now
// on, which is what its changes are read against from then on.
//
// A workspace with no commit noted before was made; one with a commit was moved, which
// changes it.
func (workspaces *Workspaces) settled(ctx context.Context, work job, result Result, used []repo.Repo) error {
	if err := workspaces.note(ctx, work.path, used); err != nil {
		return err
	}
	_, moved := workspaces.pools.Base(work.repository, work.path)
	if err := workspaces.pools.SetBase(ctx, work.repository, work.path, result.Commit); err != nil {
		return err
	}
	kind := gitdv1.WatchResponse_KIND_MADE
	if moved {
		kind = gitdv1.WatchResponse_KIND_CHANGED
	}
	workspaces.events.Tell(&gitdv1.WatchResponse{Kind: kind, Path: work.path})
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
		Args: fromPool(workspaces.pools.Path(work.repository)),
	}); err != nil {
		return Result{}, nil, git.About(err, work.repository.HTTPS())
	}
	return workspaces.settle(ctx, work)
}

// fromPool fetches a workspace's branches and tags from its pool, and nothing into its
// submodules: their pools may not hold the commits the new branches pin until bringIn
// brings them, and git's own on-demand fetch of them fails the whole update.
func fromPool(pool string) []string {
	return []string{
		"fetch", "--no-tags", "--recurse-submodules=no", "--progress", "--end-of-options", pool,
		"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*",
	}
}
