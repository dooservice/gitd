package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Publish pushes what was committed in a workspace to a new branch of its repository, or,
// asked to update, moves the branch an earlier publish made forward.
//
// Everything is staged in the pools first, and nothing is pushed when the workspace's own
// repository brings nothing new: a submodule commit nobody pointed the superproject at is not a
// change anybody can review. Then the submodules go, each to a branch of the same name in
// its own repository — its commit has to be there before the superproject's points at it —
// and the superproject last. A submodule that cannot be published stops it all, saying which.
//
// An update that brings nothing beyond what gitd published to the branch pushes nothing and
// goes on to the pull requests, so one that failed after the push is opened by publishing
// again.
//
// The branch must start with the prefix publishing is limited to, if there is one; what each
// repository brings must pass the rules its publishing settings set, checked in its pool
// before anything is pushed; and every publish, done or refused, is recorded.
func (workspaces *Workspaces) Publish(
	ctx context.Context, request *gitdv1.PublishRequest,
) (answer *gitdv1.PublishResponse, err error) {
	branch := request.GetBranch()
	entry := &gitdv1.AuditEntry{
		Workspace: request.GetPath(), Branch: branch, RequestedBy: request.GetRequestedBy(), Provenance: request.GetProvenance(),
	}
	defer func() { workspaces.record(entry, answer, err) }()
	if err := validRequester(request.GetRequestedBy()); err != nil {
		return nil, err
	}
	if err := validProvenance(request.GetProvenance()); err != nil {
		return nil, err
	}

	path, repository, release, err := workspaces.open(ctx, request.GetPath())
	if err != nil {
		return nil, err
	}
	defer release()
	entry.Url = repository.HTTPS()
	rules := workspaces.rulesFor(repository)
	if err := workspaces.validBranch(ctx, branch); err != nil {
		return nil, err
	}
	if err := branchAllowed(branch, rules.BranchPrefix); err != nil {
		return nil, err
	}
	commit, subjects, err := workspaces.pools.Stage(ctx, repository, path, branch)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			workspaces.pools.Unstage(ctx, repository, branch)
		}
	}()
	update, fresh := request.GetUpdate(), len(subjects) > 0
	if !fresh {
		if err := workspaces.nothingNew(ctx, repository, branch, update, "commit in the workspace first"); err != nil {
			return nil, err
		}
	}
	changed, err := workspaces.stageSubmodules(ctx, path, repository, branch, update)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			for _, submodule := range changed {
				workspaces.pools.Unstage(ctx, submodule.repository, branch)
			}
		}
	}()
	for _, submodule := range changed {
		if err := workspaces.checkRules(ctx, submodule.repository, branch, workspaces.rulesFor(submodule.repository)); err != nil {
			return nil, err
		}
	}
	if err := workspaces.checkRules(ctx, repository, branch, rules); err != nil {
		return nil, err
	}

	given := credentials.FromProto(request.GetCredentials())
	answer = &gitdv1.PublishResponse{Branch: branch}
	var onBranch []stagedSubmodule
	for _, submodule := range changed {
		switch {
		case !submodule.fresh:
			onBranch = append(onBranch, submodule)
		case fresh:
			if err := workspaces.pools.Push(ctx, submodule.repository, branch, given, update); err != nil {
				return nil, err
			}
			answer.Submodules = append(answer.Submodules, submodule.published)
			onBranch = append(onBranch, submodule)
		default:
			workspaces.pools.Unstage(ctx, submodule.repository, branch)
		}
	}
	if fresh {
		if err := workspaces.pools.Push(ctx, repository, branch, given, update); err != nil {
			return nil, err
		}
	}
	answer.Repository = &gitdv1.PublishedRepository{Url: repository.HTTPS(), Commit: commit, Subjects: subjects}
	workspaces.openPullRequest(ctx, request, repository, rules, onBranch, answer)
	return answer, nil
}

// nothingNew answers a publish whose repository brings nothing new: refused, unless it
// updates a branch gitd published that already holds what it would push.
func (workspaces *Workspaces) nothingNew(ctx context.Context, repository repo.Repo, branch string, update bool, hint string) error {
	if update {
		if holds, err := workspaces.pools.Holds(ctx, repository, branch); err != nil || holds {
			return err
		}
	}
	failure := git.Fail(git.NothingNew, "there is nothing new to publish: %s", hint)
	failure.Repository = repository.HTTPS()
	return failure
}
