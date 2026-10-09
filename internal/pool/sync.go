package pool

import (
	"context"
	"log/slog"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/progress"
	"github.com/dooservice/gitd/internal/repo"
)

// Sync brings what is wanted of a repository into its pool, creating the pool if it is
// not there.
//
// Each credential that opens the repository is tried, the most specific first, and then
// nothing. A failure that is not about access ends it: another credential would not change
// a timeout. What comes back is the failure of the most privileged attempt, which says
// more than "a credential is needed" after one was refused.
//
// What was brought in moments ago is taken as current: an environment with ten submodules
// of one repository, or two environments created together, would otherwise ask the
// network the same question over and over.
func (pools *Pools) Sync(
	ctx context.Context, repository repo.Repo, given []credentials.Credential, want Want, report progress.Report,
) error {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return err
	}
	defer release()
	dir := pools.Path(repository)
	existed := exists(dir)
	if existed && pools.satisfied(ctx, repository, dir, want) {
		return nil
	}
	leave, err := pools.fetches.Enter(ctx)
	if err != nil {
		failure := git.Fail(git.Busy, "too many fetches waiting")
		failure.Repository = repository.HTTPS()
		return failure
	}
	defer leave()

	if existed {
		lock.ClearStale(dir, pools.staleLock)
	}
	var reported *git.Failure
	for _, access := range pools.keys.Attempts(repository, given, false) {
		if existed {
			err = pools.fetch(ctx, dir, access, want, false, report)
		} else {
			err = pools.create(ctx, repository, access, want, report)
		}
		if err == nil {
			pools.touch(dir)
			pools.recent.note(repository, want.synced()...)
			return nil
		}
		failure, ok := git.AsFailure(err)
		if !ok {
			return err
		}
		failure.Repository = repository.HTTPS()
		failure.AccessRefused(access.Offered)
		slog.Info("pool not synced", "repository", failure.Repository, "via", access.Via, "git_said", failure.GitSaid)
		if !failure.AboutAccess() {
			return failure
		}
		if reported == nil || (access.Offered && failure.Reason != git.NotFound) {
			reported = failure
		}
	}
	return reported
}
