package workspace

import (
	"context"
	"path/filepath"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// workCommit is one side of Conflicts as a commit in the pool.
type workCommit struct {
	asked  *gitdv1.Side
	commit string
}

// Conflicts says which pairs of several works do not merge cleanly, and on which files.
// A workspace stands for what it committed: gitd brings that into the pool as publishing
// does, after the same checks of its .git, and forgets it after. Every merge is read with
// git merge-tree in the pool, without a working tree and running none of the repository's
// code.
func (workspaces *Workspaces) Conflicts(ctx context.Context, request *gitdv1.ConflictsRequest) (*gitdv1.ConflictsResponse, error) {
	if len(request.GetSides()) < 2 {
		return nil, git.Fail(git.BadAddress, "conflicts are between at least two works")
	}
	var repository *repo.Repo
	var sides []workCommit
	for _, asked := range request.GetSides() {
		commit, of, forget, err := workspaces.committedOf(ctx, asked, request.GetUrl())
		if err != nil {
			return nil, err
		}
		defer forget()
		if repository != nil && *repository != of {
			return nil, git.Fail(git.BadAddress, "the works are of different repositories")
		}
		repository = &of
		sides = append(sides, workCommit{asked: asked, commit: commit})
	}
	answer := &gitdv1.ConflictsResponse{}
	for index, first := range sides {
		for _, second := range sides[index+1:] {
			paths, err := workspaces.pools.MergeConflicts(ctx, *repository, first.commit, second.commit)
			if err != nil {
				return nil, err
			}
			if len(paths) > 0 {
				answer.Conflicts = append(answer.Conflicts, &gitdv1.Conflict{First: first.asked, Second: second.asked, Paths: paths})
			}
		}
	}
	return answer, nil
}

// committedOf is a side's commit in its repository's pool, and what forgets it there.
func (workspaces *Workspaces) committedOf(
	ctx context.Context, asked *gitdv1.Side, url string,
) (string, repo.Repo, func(), error) {
	nothing := func() {}
	if asked.GetWorkspace() == "" {
		opened, err := workspaces.openSide(ctx, asked, url, false)
		if err != nil {
			return "", repo.Repo{}, nil, err
		}
		return opened.base, opened.repository, nothing, nil
	}
	path, repository, release, err := workspaces.open(ctx, asked.GetWorkspace())
	if err != nil {
		return "", repo.Repo{}, nil, err
	}
	defer release()
	commit, forget, err := workspaces.pools.Bring(ctx, repository, filepath.Join(path, ".git"))
	if err != nil {
		return "", repo.Repo{}, nil, err
	}
	return commit, repository, forget, nil
}
