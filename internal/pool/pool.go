// Package pool keeps one bare copy of each repository on this server, and is the only
// thing that reaches the network for it.
//
// Every workspace of a repository borrows its objects from the pool through
// `objects/info/alternates`, so nothing may ever be deleted from a pool: a rollback to a
// commit that the host lost to a force push still needs it. That is why a pool is fetched
// without pruning and git's own gc is switched off in it.
package pool

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/repo"
)

// Pools are the pools under Dir. Fetches bounds how many reach the network at once, a
// git `.lock` older than staleLock is taken as left behind, and a pool synced moments ago
// is not synced again.
type Pools struct {
	Dir       string
	runner    *git.Runner
	keys      *credentials.Keys
	locks     *lock.Locks
	fetches   *lock.Queue
	staleLock time.Duration
	recent    *recentSyncs
}

// New returns the pools under dir.
func New(
	dir string,
	runner *git.Runner,
	keys *credentials.Keys,
	locks *lock.Locks,
	fetches *lock.Queue,
	staleLock time.Duration,
) *Pools {
	return &Pools{
		Dir:       dir,
		runner:    runner,
		keys:      keys,
		locks:     locks,
		fetches:   fetches,
		staleLock: staleLock,
		recent:    newRecentSyncs(10 * time.Second),
	}
}

// Address is where a repository is read over HTTPS, on its host's port.
func (pools *Pools) Address(repository repo.Repo) string {
	return pools.keys.Address(repository)
}

// Path is where the pool of a repository is, whether it exists or not.
func (pools *Pools) Path(repository repo.Repo) string {
	return repository.Dir(pools.Dir)
}

// Lock holds the pool of a repository for one write.
func (pools *Pools) Lock(ctx context.Context, repository repo.Repo) (func(), error) {
	release, err := pools.locks.Acquire(ctx, "pool:"+repository.HTTPS())
	if errors.Is(err, lock.ErrBusy) {
		failure := git.Fail(git.Busy, "another operation holds this repository")
		failure.Repository = repository.HTTPS()
		return nil, failure
	}
	return release, err
}

// Has returns the commit a revision names in the pool of a repository, if it is there.
func (pools *Pools) Has(ctx context.Context, repository repo.Repo, revision string) (string, bool, error) {
	if _, err := os.Stat(pools.Path(repository)); err != nil {
		return "", false, nil
	}
	return pools.runner.Resolve(ctx, pools.Path(repository), revision)
}

// DefaultBranch is the branch the pool's HEAD points at, as the repository named it when
// the pool was created.
func (pools *Pools) DefaultBranch(ctx context.Context, repository repo.Repo) string {
	branch, _ := pools.runner.Branch(ctx, pools.Path(repository))
	return strings.TrimSpace(branch)
}
