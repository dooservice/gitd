package pool

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Bring fetches the commit a git directory on this disk is on into the pool, under a ref
// of its own, and answers it with what forgets the ref again: for reading a workspace's
// committed work in the pool, as Conflicts does, without staging it for a publish.
func (pools *Pools) Bring(ctx context.Context, repository repo.Repo, source string) (string, func(), error) {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return "", nil, err
	}
	defer release()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", nil, err
	}
	ref := "refs/gitd/brought/" + hex.EncodeToString(random)
	dir := pools.Path(repository)
	if _, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Local: true, Args: []string{
		"fetch", "--no-prune", "--update-shallow", "--end-of-options", source, "+HEAD:" + ref,
	}}); err != nil {
		return "", nil, git.About(err, repository.HTTPS())
	}
	commit, _, err := pools.runner.Resolve(ctx, dir, ref)
	if err != nil {
		return "", nil, err
	}
	forget := func() {
		_, _ = pools.runner.Run(context.WithoutCancel(ctx), git.Cmd{Dir: dir, Args: []string{"update-ref", "-d", ref}})
	}
	return commit, forget, nil
}

// MergeConflicts says on which files merging two commits of a repository conflicts, none
// when they merge cleanly, as git merge-tree says without a working tree.
func (pools *Pools) MergeConflicts(ctx context.Context, repository repo.Repo, first, second string) ([]string, error) {
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Differences: true, Args: []string{
		"merge-tree", "--write-tree", "--name-only", "--no-messages", "-z", "--end-of-options", first, second,
	}})
	if err != nil {
		return nil, git.About(err, repository.HTTPS())
	}
	names := splitNull(out)
	if len(names) <= 1 {
		return nil, nil
	}
	return names[1:], nil
}
