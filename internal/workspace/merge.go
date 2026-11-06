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

// mergeName and mergeEmail are who a merge commit is by when the workspace names nobody:
// gitd reads no global configuration, and the account it runs as may have no name.
const (
	mergeName  = "gitd"
	mergeEmail = "gitd@localhost"
)

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
	_, present, err := workspaces.runner.Resolve(ctx, merge.path, "refs/remotes/origin/"+merge.branch)
	if err != nil {
		return Result{}, err
	}
	if !present {
		failure := git.Fail(git.NotFound, "%s is not a branch of the repository", merge.branch)
		failure.Repository = merge.repository.HTTPS()
		return Result{}, failure
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: merge.path, Env: merge.identity(ctx),
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

// identity names gitd as the merge commit's author and committer for what the workspace's
// own configuration leaves unset; what it sets, it keeps.
func (merge merger) identity(ctx context.Context) []string {
	config := filepath.Join(merge.path, ".git", "config")
	var unset []string
	if name, err := merge.workspaces.runner.Config(ctx, config, "user.name"); err != nil || name == "" {
		unset = append(unset, "GIT_AUTHOR_NAME="+mergeName, "GIT_COMMITTER_NAME="+mergeName)
	}
	if email, err := merge.workspaces.runner.Config(ctx, config, "user.email"); err != nil || email == "" {
		unset = append(unset, "GIT_AUTHOR_EMAIL="+mergeEmail, "GIT_COMMITTER_EMAIL="+mergeEmail)
	}
	return unset
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
