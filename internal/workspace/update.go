package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
)

// Update brings a workspace to the tip of a branch: the one asked for, or the one it
// follows.
func (workspaces *Workspaces) Update(
	ctx context.Context, request *gitdv1.UpdateRequest, report Report,
) (Result, error) {
	path, repository, release, err := workspaces.open(ctx, request.GetPath())
	if err != nil {
		return Result{}, err
	}
	defer release()
	branch := request.GetBranch()
	if branch == "" {
		if branch, err = workspaces.following(ctx, path); err != nil {
			return Result{}, err
		}
	}
	work := job{
		path:       path,
		repository: repository,
		revision:   onBranch(branch),
		submodules: request.GetWithSubmodules(),
		full:       workspaces.keepsHistory(ctx, path),
		given:      credentials.FromProto(request.GetCredentials()),
		report:     report,
	}
	if err := workspaces.pools.Sync(ctx, repository, work.given, work.want(), reporter(report, repository)); err != nil {
		return Result{}, err
	}
	return workspaces.moveTo(ctx, work)
}

// Checkout moves a workspace to a commit, as a rollback does. The pool is only synced
// when the commit is not in it already, which it is for anything the workspace was at.
func (workspaces *Workspaces) Checkout(
	ctx context.Context, request *gitdv1.CheckoutRequest, report Report,
) (Result, error) {
	if request.GetCommit() == "" {
		return Result{}, git.Fail(git.BadAddress, "say which commit to check out")
	}
	path, repository, release, err := workspaces.open(ctx, request.GetPath())
	if err != nil {
		return Result{}, err
	}
	defer release()
	work := job{
		path:       path,
		repository: repository,
		revision:   atCommit(request.GetCommit()),
		submodules: request.GetWithSubmodules(),
		full:       workspaces.keepsHistory(ctx, path),
		given:      credentials.FromProto(request.GetCredentials()),
		report:     report,
	}
	_, present, err := workspaces.runner.Resolve(ctx, path, request.GetCommit())
	if err != nil {
		return Result{}, err
	}
	if !present {
		if err := workspaces.pools.Sync(ctx, repository, work.given, work.want(), reporter(report, repository)); err != nil {
			return Result{}, err
		}
	}
	return workspaces.moveTo(ctx, work)
}

// moveTo brings a workspace up to its pool and settles it at the job's revision, over
// whatever was left on disk. A job that keeps the whole history notes it, so the updates
// after it keep it too.
func (workspaces *Workspaces) moveTo(ctx context.Context, work job) (Result, error) {
	if work.full {
		if err := workspaces.runner.SetConfig(ctx, work.path, "gitd.history", "full"); err != nil {
			return Result{}, err
		}
	}
	result, used, err := workspaces.bringUp(ctx, work)
	if err != nil {
		return Result{}, err
	}
	result.Strategy, result.Layers = workspaces.made(ctx, work.path)
	return result, workspaces.note(ctx, work.path, used)
}
