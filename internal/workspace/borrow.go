package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// borrow points the git directory gitDir at its repository's pool: its `alternates` names
// the pool's objects, and its `shallow` is the pool's, so that both say the same thing
// about where the history stops. A pool with the whole history leaves no `shallow` behind.
//
// It is done before every fetch from the pool, because a sync may have moved where the
// pool's history stops.
func (workspaces *Workspaces) borrow(gitDir string, repository repo.Repo) error {
	alternates := filepath.Join(gitDir, "objects", "info", "alternates")
	if err := os.MkdirAll(filepath.Dir(alternates), 0o755); err != nil {
		return err
	}
	objects := filepath.Join(workspaces.pools.Path(repository), "objects") + "\n"
	if err := pool.WriteAtomic(alternates, []byte(objects)); err != nil {
		return err
	}
	shallow := filepath.Join(gitDir, "shallow")
	from := workspaces.pools.Shallow(repository)
	if from == "" {
		if err := os.Remove(shallow); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return pool.WriteAtomic(shallow, data)
}

// keepsHistory is whether the workspace at path was made with the repository's whole
// history, which every later sync of it keeps.
func (workspaces *Workspaces) keepsHistory(ctx context.Context, path string) bool {
	history, _ := workspaces.runner.Config(ctx, filepath.Join(path, ".git", "config"), "gitd.history")
	return history == "full"
}
