package workspace

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/repo"
)

// borrow points the git directory gitDir at its repository's pool: its `alternates` names
// the pool's objects, and its `shallow` is the pool's, so that both say the same thing
// about where the history stops. A pool with the whole history leaves no `shallow` behind.
//
// It is done before every fetch from the pool, because a sync may have moved where the
// pool's history stops.
//
// Both are written through an os.Root of the git directory, so a link a sandbox left in it
// cannot carry the writes elsewhere.
func (workspaces *Workspaces) borrow(gitDir string, repository repo.Repo) error {
	dir, err := openGitDir(gitDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	objects := filepath.Join(workspaces.pools.Path(repository), "objects") + "\n"
	if err := writeInside(dir, filepath.Join("objects", "info", "alternates"), []byte(objects)); err != nil {
		return err
	}
	from := workspaces.pools.Shallow(repository)
	if from == "" {
		return removeInside(dir, "shallow")
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return writeInside(dir, "shallow", data)
}

// keepsHistory is whether the workspace at path was made with the repository's whole
// history, which every later sync of it keeps.
func (workspaces *Workspaces) keepsHistory(ctx context.Context, path string) bool {
	history, _ := workspaces.runner.Config(ctx, filepath.Join(path, ".git", "config"), "gitd.history")
	return history == "full"
}
