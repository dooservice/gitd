package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
)

// Publish pushes what was committed in a workspace to a new branch of its repository.
//
// Everything is staged in the pools first, and nothing is pushed when the workspace's own
// repository brings nothing new: a submodule commit nobody pointed the superproject at is not a
// change anybody can review. Then the submodules go, each to a branch of the same name in
// its own repository — its commit has to be there before the superproject's points at it —
// and the superproject last. A submodule that cannot be published stops it all, saying which.
//
// The branch must start with the prefix publishing is limited to, if there is one, and every
// publish, done or refused, is recorded.
func (workspaces *Workspaces) Publish(
	ctx context.Context, request *gitdv1.PublishRequest,
) (answer *gitdv1.PublishResponse, err error) {
	branch := request.GetBranch()
	entry := &gitdv1.AuditEntry{Workspace: request.GetPath(), Branch: branch}
	defer func() { workspaces.record(entry, answer, err) }()

	path, repository, release, err := workspaces.open(ctx, request.GetPath())
	if err != nil {
		return nil, err
	}
	defer release()
	entry.Url = repository.HTTPS()
	validated := git.Cmd{Dir: "/", Args: []string{"check-ref-format", "--branch", branch}}
	if _, err := workspaces.runner.Run(ctx, validated); err != nil {
		return nil, git.Fail(git.BadAddress, "%q is not a branch name", branch)
	}
	if err := workspaces.branchAllowed(branch); err != nil {
		return nil, err
	}
	commit, subjects, err := workspaces.pools.Stage(ctx, repository, path, branch)
	if err != nil {
		return nil, err
	}
	if len(subjects) == 0 {
		failure := git.Fail(git.NothingNew, "there is nothing new to publish: commit in the workspace first")
		failure.Repository = repository.HTTPS()
		return nil, failure
	}
	changed, err := workspaces.stageSubmodules(ctx, path, repository, branch)
	if err != nil {
		return nil, err
	}

	given := credentials.FromProto(request.GetCredentials())
	answer = &gitdv1.PublishResponse{Branch: branch}
	for _, submodule := range changed {
		if err := workspaces.pools.Push(ctx, submodule.repository, branch, given); err != nil {
			return nil, err
		}
		answer.Submodules = append(answer.Submodules, submodule.published)
	}
	if err := workspaces.pools.Push(ctx, repository, branch, given); err != nil {
		return nil, err
	}
	answer.Repository = &gitdv1.PublishedRepository{Url: repository.HTTPS(), Commit: commit, Subjects: subjects}
	return answer, nil
}
