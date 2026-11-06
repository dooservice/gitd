package workspace

import (
	"context"
	"errors"
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
// The pool brings the branch with gitd's credential, and with its history back to when the
// workspace was put on its commit, the commit both branches most likely share and one the
// workspace holds already; the workspace then fetches the branch from the pool, on this
// disk, and merges. When git finds no
// commit shared in that much history, the whole history is brought and the merge tried
// again. Nothing is forced: the branch only gains a merge commit, so a branch published
// from it before still moves forward.
type merger struct {
	workspaces *Workspaces
	path       string
	repository repo.Repo
	branch     string
	given      []credentials.Credential
	report     Report
}

// unrelated is what git says when it finds no commit two branches share.
const unrelated = "refusing to merge unrelated histories"

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
	merged := errors.New(unrelated)
	if since, bounded := merge.since(ctx); bounded {
		merged = merge.attempt(ctx, pool.BranchSince(merge.branch, since), "")
	}
	if merged != nil && strings.Contains(merged.Error(), unrelated) {
		merged = merge.attempt(ctx, pool.Everything(), "--unshallow")
	}
	if merged != nil {
		return Result{}, merge.stopped(ctx, onto, merged)
	}
	commit, _, err := workspaces.runner.Resolve(ctx, merge.path, "HEAD")
	if err != nil {
		return Result{}, err
	}
	strategy, layers := workspaces.made(ctx, merge.path)
	return Result{Commit: commit, Branch: onto, Strategy: strategy, Layers: layers}, nil
}

// since is when the commit the workspace was put on was made, as git's --shallow-since
// reads it; unknown for a workspace gitd noted no commit of.
func (merge merger) since(ctx context.Context) (string, bool) {
	base, noted := merge.workspaces.pools.Base(merge.repository, merge.path)
	if !noted {
		return "", false
	}
	out, err := merge.workspaces.runner.Run(ctx, git.Cmd{Dir: merge.path, Args: []string{"log", "-1", "--format=%cI", base, "--"}})
	when := strings.TrimSpace(out)
	return when, err == nil && when != ""
}

// attempt brings what want says into the pool and the workspace, deepening a shallow
// workspace as deepen says when it says, and merges.
func (merge merger) attempt(ctx context.Context, want pool.Want, deepen string) error {
	workspaces := merge.workspaces
	if err := workspaces.pools.Sync(ctx, merge.repository, merge.given, want, reporter(merge.report, merge.repository)); err != nil {
		return err
	}
	if err := merge.fetch(ctx, deepen); err != nil {
		return err
	}
	_, present, err := workspaces.runner.Resolve(ctx, merge.path, "refs/remotes/origin/"+merge.branch)
	if err != nil {
		return err
	}
	if !present {
		failure := git.Fail(git.NotFound, "%s is not a branch of the repository", merge.branch)
		failure.Repository = merge.repository.HTTPS()
		return failure
	}
	_, err = workspaces.runner.Run(ctx, git.Cmd{
		Dir: merge.path, Env: merge.identity(ctx),
		Args: []string{"merge", "--no-edit", "--end-of-options", "refs/remotes/origin/" + merge.branch},
	})
	return err
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

// fetch brings the pool's branches into the workspace, deepening it as deepen says, when
// it says and the workspace was made with its tip only. A workspace that holds the whole history after it is noted
// so, for the updates after it.
func (merge merger) fetch(ctx context.Context, deepen string) error {
	args := fromPool(merge.workspaces.pools.Path(merge.repository))
	if _, err := os.Stat(filepath.Join(merge.path, ".git", "shallow")); err == nil && deepen != "" {
		args = append(args[:1], append([]string{deepen}, args[1:]...)...)
	}
	_, err := merge.workspaces.runner.Run(ctx, git.Cmd{
		Dir: merge.path, Local: true, Progress: reporter(merge.report, merge.repository), Args: args,
	})
	if err != nil {
		return git.About(err, merge.repository.HTTPS())
	}
	if _, err := os.Stat(filepath.Join(merge.path, ".git", "shallow")); err == nil {
		return nil
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
