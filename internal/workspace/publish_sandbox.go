package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// PublishPushed publishes a branch a sandbox pushed to its sandbox repository, at source,
// through the endpoint: under the same name, with the same branch prefix, rules, pull
// request and audit log as publishing a workspace. A sandbox repository has no submodules
// of gitd's and no configuration a sandbox wrote — smart HTTP writes neither — so nothing of
// it needs the checks a workspace's .git does.
func (workspaces *Workspaces) PublishPushed(
	ctx context.Context, request *gitdv1.PublishRequest, repository repo.Repo, source string,
) (answer *gitdv1.PublishResponse, err error) {
	branch := request.GetBranch()
	entry := &gitdv1.AuditEntry{
		Sandbox: request.GetSandbox(), Url: repository.HTTPS(), Branch: branch,
		RequestedBy: request.GetRequestedBy(), Provenance: request.GetProvenance(),
	}
	defer func() { workspaces.record(entry, answer, err) }()
	if err := validRequester(request.GetRequestedBy()); err != nil {
		return nil, err
	}
	if err := validProvenance(request.GetProvenance()); err != nil {
		return nil, err
	}
	rules := workspaces.rulesFor(repository)
	if err := workspaces.validBranch(ctx, branch); err != nil {
		return nil, err
	}
	if err := branchAllowed(branch, rules.BranchPrefix); err != nil {
		return nil, err
	}
	if _, pushed, err := workspaces.runner.Resolve(ctx, source, "refs/heads/"+branch); err != nil || !pushed {
		failure := git.Fail(git.NotFound, "the sandbox pushed no branch %s", branch)
		failure.Repository = repository.HTTPS()
		return nil, failure
	}
	commit, subjects, err := workspaces.pools.StageRef(ctx, repository, source, "refs/heads/"+branch, branch)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			workspaces.pools.Unstage(ctx, repository, branch)
		}
	}()
	if len(subjects) == 0 {
		failure := git.Fail(git.NothingNew, "there is nothing new to publish: the sandbox pushed no new commit")
		failure.Repository = repository.HTTPS()
		return nil, failure
	}
	if err := workspaces.checkRules(ctx, repository, branch, rules); err != nil {
		return nil, err
	}
	account, err := workspaces.accountOf(ctx, repository, request, rules)
	if err != nil {
		return nil, err
	}
	if err := workspaces.pools.Push(ctx, repository, branch, credentials.FromProto(request.GetCredentials())); err != nil {
		return nil, err
	}
	answer = &gitdv1.PublishResponse{Branch: branch, Repository: &gitdv1.PublishedRepository{
		Url: repository.HTTPS(), Commit: commit, Subjects: subjects,
	}}
	workspaces.openPullRequest(ctx, request, repository, rules, account, answer)
	return answer, nil
}
