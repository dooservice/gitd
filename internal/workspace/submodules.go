package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// SyncSubmodules checks out every submodule a workspace declares, or the one of a URL.
func (workspaces *Workspaces) SyncSubmodules(
	ctx context.Context, request *gitdv1.SyncSubmodulesRequest, report Report,
) (*gitdv1.SyncSubmodulesResult, error) {
	path, repository, release, err := workspaces.open(ctx, request.GetPath())
	if err != nil {
		return nil, err
	}
	defer release()
	work := job{path: path, repository: repository, given: credentials.FromProto(request.GetCredentials()), report: report}
	pending, used, err := workspaces.submodules(ctx, work, request.GetOnlyUrl())
	if err != nil {
		return nil, err
	}
	if err := workspaces.note(ctx, path, used); err != nil {
		return nil, err
	}
	states, err := workspaces.states(ctx, path)
	return &gitdv1.SyncSubmodulesResult{Submodules: states, PendingSubmodules: pending}, err
}

// submodules brings each submodule of the job's workspace in from its own pool, at the
// commit the superproject pinned. One that cannot be brought in is returned as pending
// and the rest go on: a submodule nothing opens should not keep the others out. Nested
// submodules are not brought in.
func (workspaces *Workspaces) submodules(
	ctx context.Context, work job, onlyURL string,
) ([]*gitdv1.PendingSubmodule, []repo.Repo, error) {
	only, ok := repo.Of(onlyURL)
	if onlyURL != "" && !ok {
		return nil, nil, git.Unnamed(onlyURL)
	}
	declared, err := workspaces.declaredIn(ctx, work.path)
	if err != nil {
		return nil, nil, err
	}
	pins, err := workspaces.pinned(ctx, work.path, declared)
	if err != nil {
		return nil, nil, err
	}
	var pending []*gitdv1.PendingSubmodule
	var used []repo.Repo
	for _, submodule := range declared {
		repository, named := repo.Of(repo.Relative(submodule.url, work.repository))
		commit, pinned := pins[submodule.path]
		if !pinned || (onlyURL != "" && repository != only) {
			continue
		}
		if !named {
			pending = append(pending, submodule.pending(git.Unnamed(submodule.url)))
			continue
		}
		failure, err := workspaces.bringIn(ctx, work, submodule, repository, commit)
		if err != nil {
			return nil, nil, err
		}
		if failure != nil {
			pending = append(pending, submodule.pending(failure))
			continue
		}
		used = append(used, repository)
	}
	return pending, used, nil
}
