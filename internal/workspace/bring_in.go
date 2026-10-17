package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// bringIn checks one submodule out from its pool at the commit the superproject pinned,
// bringing only that commit into the pool when it is not there: a submodule is what it is
// pinned at, and its history is not needed to check it out.
//
// A failure about that submodule comes back as a failure, to be reported as pending;
// anything else is an error that stops the whole operation.
func (workspaces *Workspaces) bringIn(
	ctx context.Context, work job, submodule submodule, repository repo.Repo, commit string,
) (*git.Failure, error) {
	asPending := func(err error) (*git.Failure, error) {
		if failure, ok := git.AsFailure(err); ok && failure.Reason != git.Timeout {
			failure.Repository = repository.HTTPS()
			return failure, nil
		}
		return nil, err
	}
	present, err := workspaces.pinnedIsInPool(ctx, work, repository, commit)
	if err != nil {
		return asPending(err)
	}
	if !present {
		failure := git.Fail(git.NotFound, "commit %s pinned at %s is not in the repository", commit, submodule.path)
		failure.Repository = repository.HTTPS()
		return failure, nil
	}
	if work.report != nil {
		work.report(&gitdv1.Progress{Phase: "Submodule " + submodule.path, Percent: -1, Repository: repository.HTTPS()})
	}
	if err := workspaces.mount(ctx, work, submodule, repository, commit); err != nil {
		return asPending(err)
	}
	return nil, nil
}

func (workspaces *Workspaces) pinnedIsInPool(
	ctx context.Context, work job, repository repo.Repo, commit string,
) (bool, error) {
	if _, present, err := workspaces.pools.Has(ctx, repository, commit); err != nil || present {
		return present, err
	}
	report := reporter(work.report, repository)
	if err := workspaces.pools.Sync(ctx, repository, work.given, pool.OnlyCommit(commit), report); err != nil {
		return false, err
	}
	_, present, err := workspaces.pools.Has(ctx, repository, commit)
	return present, err
}
