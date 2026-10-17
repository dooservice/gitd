package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/repo"
)

// open holds a workspace gitd made, for a write.
func (workspaces *Workspaces) open(ctx context.Context, path string) (string, repo.Repo, func(), error) {
	path, err := workspaces.allowed(path)
	if err != nil {
		return "", repo.Repo{}, nil, err
	}
	release, err := workspaces.hold(ctx, path)
	if err != nil {
		return "", repo.Repo{}, nil, err
	}
	held, err := workspaces.madeFor(ctx, path)
	if err == nil && held == nil {
		err = git.Fail(git.PathRefused, "%s is not a workspace gitd made; adopt it first", path)
	}
	if err != nil {
		release()
		return "", repo.Repo{}, nil, err
	}
	return path, *held, release, nil
}

// hold takes a place in the queue of writes and the workspace's lock, and clears what a
// killed git left behind there.
func (workspaces *Workspaces) hold(ctx context.Context, path string) (func(), error) {
	leave, err := workspaces.writes.Enter(ctx)
	if errors.Is(err, lock.ErrBusy) {
		return nil, git.Fail(git.Busy, "too many operations waiting")
	}
	if err != nil {
		return nil, err
	}
	release, err := workspaces.locks.Acquire(ctx, "workspace:"+path)
	if err != nil {
		leave()
		if errors.Is(err, lock.ErrBusy) {
			return nil, git.Fail(git.Busy, "another operation holds %s", path)
		}
		return nil, err
	}
	lock.ClearStale(filepath.Join(path, ".git"), workspaces.staleLock)
	return func() {
		release()
		leave()
	}, nil
}

// madeFor is the repository of the workspace gitd made at path, or nil when gitd did not
// make what is there.
func (workspaces *Workspaces) madeFor(ctx context.Context, path string) (*repo.Repo, error) {
	config := filepath.Join(path, ".git", "config")
	if _, err := os.Stat(config); err != nil {
		return nil, nil
	}
	address, err := workspaces.runner.Config(ctx, config, "gitd.url")
	if err != nil || address == "" {
		return nil, err
	}
	repository, ok := repo.Of(address)
	if !ok {
		return nil, nil
	}
	return &repository, nil
}
