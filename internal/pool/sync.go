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
// It is read with the one credential that opens it, or with nothing: which credential
// that is was decided by whoever asked, so a refusal is the answer and not a reason to
// try another.
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
	access := pools.keys.For(repository, given)
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
	return failure
}
