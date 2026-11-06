package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// mergeIdentity is who a merge commit is by when the workspace names nobody: git takes
// EMAIL only when user.email is not set, and the name from the account gitd runs as.
const mergeIdentity = "EMAIL=gitd@localhost"

// merger merges a branch of a workspace's repository into the branch the workspace is on.
// The pool brings the branch with gitd's credential and the whole history, which finding
// the commit both branches share needs; the workspace then fetches it from the pool, on
// this disk, and merges. Nothing is forced: the branch only gains a merge commit, so a
// branch published from it before still moves forward.
type merger struct {
	workspaces *Workspaces
	path       string
	repository repo.Repo
	branch     string
	given      []credentials.Credential
	report     Report
}

func (merge merger) run(ctx context.Context) (Result, error) {
	workspaces := merge.workspaces
	if err := workspaces.validBranch(ctx, merge.branch); err != nil {
		return Result{}, err
	}
	onto, err := workspaces.onOwnBranch(ctx, merge.path)
	if err != nil {
		return Result{}, err
	}
	if err := workspaces.guard(ctx, merge.path); err != nil {
		return Result{}, err
	}
	if err := workspaces.pools.Sync(ctx, merge.repository, merge.given, pool.Everything(), reporter(merge.report, merge.repository)); err != nil {
		return Result{}, err
	}
	if err := merge.fetch(ctx); err != nil {
		return Result{}, err
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: merge.path, Env: []string{mergeIdentity},
		Args: []string{"merge", "--no-edit", "--end-of-options", "refs/remotes/origin/" + merge.branch},
	}); err != nil {
		return Result{}, merge.stopped(ctx, onto, err)
	}
	commit, _, err := workspaces.runner.Resolve(ctx, merge.path, "HEAD")
	if err != nil {
		return Result{}, err
	}
	strategy, layers := workspaces.made(ctx, merge.path)
	return Result{Commit: commit, Branch: onto, Strategy: strategy, Layers: layers}, nil
}

// fetch brings the pool's branches into the workspace, and the whole history when the
// workspace was made with its tip only.
func (merge merger) fetch(ctx context.Context) error {
	args := fromPool(merge.workspaces.pools.Path(merge.repository))
	if _, err := os.Stat(filepath.Join(merge.path, ".git", "shallow")); err == nil {
		args = append(args[:1], append([]string{"--unshallow"}, args[1:]...)...)
	}
	_, err := merge.workspaces.runner.Run(ctx, git.Cmd{
		Dir: merge.path, Local: true, Progress: reporter(merge.report, merge.repository), Args: args,
	})
	if err != nil {
		return git.About(err, merge.repository.HTTPS())
	}
	return merge.workspaces.runner.SetConfig(ctx, merge.path, "gitd.history", "full")
}

// stopped is why a merge did not finish: conflicts, which stay marked in the workspace for
// whoever works in it, or git failing otherwise.
func (merge merger) stopped(ctx context.Context, onto string, err error) error {
	unmerged, listed := merge.workspaces.runner.Run(ctx, git.Cmd{
		Dir: merge.path, Args: []string{"diff", "--name-only", "--diff-filter=U"},
	})
	files := strings.Fields(unmerged)
	if listed != nil || len(files) == 0 {
		return err
	}
	failure := git.Fail(git.Conflict, "merging %s into %s stopped on conflicts in %s: resolve them in the workspace and commit",
		merge.branch, onto, strings.Join(files, ", "))
	failure.Repository = merge.repository.HTTPS()
	return failure
}
