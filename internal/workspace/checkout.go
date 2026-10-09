package workspace

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/git"
)

// checkout puts the working tree at the job's revision. A branch is checked out as itself,
// following origin; a commit is checked out detached, and the branch followed so far is
// kept for the next update. No revision at all is the repository's default branch.
func (workspaces *Workspaces) checkout(ctx context.Context, work job) (Result, error) {
	if commit := work.revision.GetCommit(); commit != "" {
		return workspaces.checkoutCommit(ctx, work, commit)
	}
	branch := work.revision.GetBranch()
	if branch == "" {
		branch = workspaces.pools.DefaultBranch(ctx, work.repository)
	}
	return workspaces.checkoutBranch(ctx, work, branch)
}

func (workspaces *Workspaces) checkoutCommit(ctx context.Context, work job, commit string) (Result, error) {
	sha, present, err := workspaces.runner.Resolve(ctx, work.path, commit)
	if err != nil {
		return Result{}, err
	}
	if !present {
		return Result{}, notIn(work, "commit "+commit)
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: work.path, Progress: reporter(work.report, work.repository),
		Args: []string{"checkout", "--force", "--progress", "--detach", sha},
	}); err != nil {
		return Result{}, git.About(err, work.repository.HTTPS())
	}
	followed, _ := workspaces.runner.Config(ctx, filepath.Join(work.path, ".git", "config"), "gitd.branch")
	return Result{Commit: sha, Branch: followed}, nil
}

func (workspaces *Workspaces) checkoutBranch(ctx context.Context, work job, branch string) (Result, error) {
	upstream := "refs/remotes/origin/" + branch
	sha, present, err := workspaces.runner.Resolve(ctx, work.path, upstream)
	if err != nil {
		return Result{}, err
	}
	if !present || branch == "" || strings.HasPrefix(branch, "-") {
		return Result{}, notIn(work, "branch "+branch)
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: work.path, Progress: reporter(work.report, work.repository),
		Args: []string{"checkout", "--force", "--progress", "-B", branch, upstream},
	}); err != nil {
		return Result{}, git.About(err, work.repository.HTTPS())
	}
	if err := workspaces.runner.SetConfig(ctx, work.path, "gitd.branch", branch); err != nil {
		return Result{}, err
	}
	return Result{Commit: sha, Branch: branch}, nil
}

// following is the branch a workspace is on, or the one it followed before a rollback
// detached it.
func (workspaces *Workspaces) following(ctx context.Context, path string) (string, error) {
	if branch, on := workspaces.runner.Branch(ctx, path); on {
		return branch, nil
	}
	followed, err := workspaces.runner.Config(ctx, filepath.Join(path, ".git", "config"), "gitd.branch")
	if err == nil && followed == "" {
		err = git.Fail(git.NotFound, "%s is on no branch and follows none; say which one", path)
	}
	return followed, err
}

func notIn(work job, what string) *git.Failure {
	failure := git.Fail(git.NotFound, "%s is not in the repository", what)
	failure.Repository = work.repository.HTTPS()
	return failure
}
