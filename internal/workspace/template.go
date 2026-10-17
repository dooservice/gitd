package workspace

import (
	"context"
	"path/filepath"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/template"
)

// BuildTemplate checks a template out at building, as a workspace is made: borrowing from
// the pool, detached at the key's commit, with its submodules when the key asks. Its pool
// must hold the commit already. It is tuned for the copies made from it — whose inodes and
// change times differ from what its index recorded — to compare only sizes and
// modification times, so that their first `git status` reads nothing. It is noted on its
// pools as final, where it will live.
func (workspaces *Workspaces) BuildTemplate(ctx context.Context, key template.Key, building, final string) error {
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: filepath.Dir(building), Args: []string{"init", "--quiet", building},
	}); err != nil {
		return err
	}
	if err := workspaces.runner.SetConfig(ctx, building,
		"remote.origin.url", workspaces.pools.Address(key.Repository),
		"gitd.url", key.Repository.HTTPS(),
		"gitd.pool", workspaces.pools.Path(key.Repository),
		"gitd.history", "tips",
		"core.checkStat", "minimal",
		"core.trustctime", "false",
	); err != nil {
		return err
	}
	work := job{path: building, repository: key.Repository, revision: atCommit(key.Commit), submodules: key.Submodules}
	_, used, err := workspaces.bringUp(ctx, work)
	if err != nil {
		return err
	}
	return workspaces.note(ctx, final, used)
}

// ForgetTemplate takes the template at dir off the pools it borrows from: its repository's
// and its submodules'.
func (workspaces *Workspaces) ForgetTemplate(ctx context.Context, key template.Key, dir string) error {
	borrowed := []repo.Repo{key.Repository}
	if key.Submodules {
		declared, _, err := workspaces.declaredIn(ctx, dir)
		if err != nil {
			return err
		}
		for _, submodule := range declared {
			if repository, ok := repo.Of(repo.Relative(submodule.url, key.Repository)); ok {
				borrowed = append(borrowed, repository)
			}
		}
	}
	for _, repository := range borrowed {
		if err := workspaces.pools.Forget(ctx, repository, dir); err != nil {
			return err
		}
	}
	return nil
}
