package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Prepare makes a workspace of a repository at a path, at a branch, a commit, or the
// repository's default branch. A workspace gitd already made there for the same
// repository is moved to the revision instead.
func (workspaces *Workspaces) Prepare(
	ctx context.Context, request *gitdv1.PrepareRequest, report Report,
) (Result, error) {
	repository, ok := repo.Of(request.GetUrl())
	if !ok {
		return Result{}, git.Unnamed(request.GetUrl())
	}
	path, err := workspaces.allowed(request.GetPath())
	if err != nil {
		return Result{}, err
	}
	release, err := workspaces.hold(ctx, path)
	if err != nil {
		return Result{}, err
	}
	defer release()
	held, err := workspaces.madeFor(ctx, path)
	if err != nil {
		return Result{}, err
	}
	if held != nil && *held != repository {
		return Result{}, git.Fail(git.PathRefused,
			"%s is a workspace of %s, not of %s", path, held.HTTPS(), repository.HTTPS())
	}
	if held == nil && !vacant(path) {
		return Result{}, git.Fail(git.PathRefused,
			"%s already holds something gitd did not make; adopt it or remove it first", path)
	}
	work := job{
		path:       path,
		repository: repository,
		revision:   request.GetRevision(),
		submodules: request.GetWithSubmodules(),
		full:       request.GetFullHistory() || held != nil && workspaces.keepsHistory(ctx, path),
		given:      credentials.FromProto(request.GetCredentials()),
		report:     report,
	}
	if err := workspaces.pools.Sync(ctx, repository, work.given, work.want(), reporter(report, repository)); err != nil {
		return Result{}, err
	}
	if held != nil {
		return workspaces.moveTo(ctx, work)
	}
	return workspaces.create(ctx, work)
}
