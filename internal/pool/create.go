package pool

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/progress"
	"github.com/dooservice/gitd/internal/repo"
)

func (pools *Pools) create(
	ctx context.Context, repository repo.Repo, access credentials.Access, want Want, report progress.Report,
) error {
	return pools.build(ctx, repository, func(temp string) error {
		return pools.fetch(ctx, temp, access, want, true, report)
	})
}

// Seed creates the pool of a repository from a clone already on this disk, without the
// network, when there is no pool yet: the branches the clone last saw on origin, and the
// commit it is on. A shallow clone seeds a shallow pool.
func (pools *Pools) Seed(ctx context.Context, repository repo.Repo, clone string, report progress.Report) error {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return err
	}
	defer release()
	if exists(pools.Path(repository)) {
		return nil
	}
	refspecs := []string{"+refs/remotes/origin/*:refs/heads/*", "^refs/remotes/origin/HEAD", "+refs/tags/*:refs/tags/*"}
	if head, on, err := pools.runner.Resolve(ctx, clone, "HEAD"); err == nil && on {
		refspecs = append(refspecs, head+":refs/gitd/commits/"+head)
	}
	return pools.build(ctx, repository, func(temp string) error {
		_, err := pools.runner.Run(ctx, git.Cmd{
			Dir: temp, Local: true, Progress: report,
			Args: append([]string{"fetch", "--no-prune", "--update-shallow", "--end-of-options", clone}, refspecs...),
		})
		return err
	})
}

// build makes the pool of a repository in a temporary directory — configured first, so
// that nothing is ever pruned from it, then filled — and moves it into place once it is
// complete.
func (pools *Pools) build(ctx context.Context, repository repo.Repo, fill func(temp string) error) error {
	dir := pools.Path(repository)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	temp := Temp(dir)
	defer os.RemoveAll(temp)
	initialize := git.Cmd{Dir: filepath.Dir(dir), Args: []string{"init", "--bare", "--quiet", temp}}
	if _, err := pools.runner.Run(ctx, initialize); err != nil {
		return err
	}
	if err := pools.runner.SetConfig(ctx, temp,
		"remote.origin.url", repository.HTTPS(),
		"gc.auto", "0",
		"gc.pruneExpire", "never",
		"gc.reflogExpireUnreachable", "never",
		"core.logAllRefUpdates", "false",
	); err != nil {
		return err
	}
	if err := fill(temp); err != nil {
		return err
	}
	if err := os.Rename(temp, dir); err != nil {
		return err
	}
	pools.touch(dir)
	return nil
}

// Shallow is the path of the file that says where the pool's history stops, or "" when
// it holds the whole of it. A workspace borrowing from a shallow pool needs the same file.
func (pools *Pools) Shallow(repository repo.Repo) string {
	path := filepath.Join(pools.Path(repository), "shallow")
	if !exists(path) {
		return ""
	}
	return path
}
