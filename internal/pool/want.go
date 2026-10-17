package pool

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/dooservice/gitd/internal/repo"
)

// Want is what a sync must bring into a pool: the repository's whole history, or only some
// branch tips and some commits, each without the history behind it. A want with neither
// branches nor commits is the repository's default branch.
//
// Only what is checked out, by default: a large repository as a submodule is its pinned
// commit, a few hundred megabytes, and not its history, which may be several gigabytes. A
// workspace that wants `git log` and `git blame` to go back as far as in a clone asks for
// the whole history.
type Want struct {
	Full     bool
	Branches []string
	Commits  []string
}

// Everything is the whole history of every branch.
func Everything() Want {
	return Want{Full: true}
}

// OnlyBranch is the tip of one branch, or of the default one when branch is empty.
func OnlyBranch(branch string) Want {
	if branch == "" {
		return Want{}
	}
	return Want{Branches: []string{branch}}
}

// OnlyCommit is one commit and what it points at.
func OnlyCommit(commit string) Want {
	return Want{Commits: []string{commit}}
}

// satisfied is whether the pool at dir already holds what is wanted, recently enough.
func (pools *Pools) satisfied(ctx context.Context, repository repo.Repo, dir string, want Want) bool {
	if want.Full {
		return !exists(filepath.Join(dir, "shallow")) && pools.recent.current(repository, "*")
	}
	if len(want.Branches) == 0 && len(want.Commits) == 0 {
		return pools.recent.current(repository, "HEAD")
	}
	for _, branch := range want.Branches {
		if !pools.recent.current(repository, branch) {
			return false
		}
	}
	return !slices.ContainsFunc(want.Commits, func(commit string) bool {
		_, present, err := pools.runner.Resolve(ctx, dir, commit)
		return err != nil || !present
	})
}

func (want Want) synced() []string {
	switch {
	case want.Full:
		return []string{"*", "HEAD"}
	case len(want.Branches) == 0 && len(want.Commits) == 0:
		return []string{"HEAD"}
	default:
		return want.Branches
	}
}
