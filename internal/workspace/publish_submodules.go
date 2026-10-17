package workspace

import (
	"context"
	"path/filepath"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

// stagedSubmodule is a submodule staged to be pushed: its repository, and what the answer
// says of it.
type stagedSubmodule struct {
	repository repo.Repo
	published  *gitdv1.PublishedRepository
}

// stageSubmodules stages every submodule that was committed to in the workspace: one whose
// commit its repository's branches do not have. One that is as it came is left alone.
func (workspaces *Workspaces) stageSubmodules(
	ctx context.Context, path string, parent repo.Repo, branch string,
) ([]stagedSubmodule, error) {
	declared, _, err := workspaces.declaredIn(ctx, path)
	if err != nil {
		return nil, err
	}
	var changed []stagedSubmodule
	for _, submodule := range declared {
		workTree := filepath.Join(path, submodule.path)
		repository, named := repo.Of(repo.Relative(submodule.url, parent))
		if !named || !exists(filepath.Join(workTree, ".git")) {
			continue
		}
		gitDir := filepath.Join(path, ".git", "modules", submodule.name)
		commit, subjects, err := workspaces.pools.Stage(ctx, repository, gitDir, branch)
		if err != nil {
			return nil, err
		}
		if len(subjects) > 0 {
			changed = append(changed, stagedSubmodule{repository: repository, published: &gitdv1.PublishedRepository{
				Url: repository.HTTPS(), Path: submodule.path, Commit: commit, Subjects: subjects,
			}})
		}
	}
	return changed, nil
}
