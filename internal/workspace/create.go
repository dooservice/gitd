package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/strategy"
)

// create makes the workspace in a temporary directory next to where it goes, borrowing
// from the pool, checks it out there, and renames it into place.
//
// It is made with `init` and not `clone --shared`: git ignores `--shared` for a shallow
// pool and copies every object, which is the disk the pool is there to save.
func (workspaces *Workspaces) create(ctx context.Context, work job) (Result, error) {
	if err := os.MkdirAll(filepath.Dir(work.path), 0o755); err != nil {
		return Result{}, err
	}
	building := work
	building.path = pool.Temp(work.path)
	defer os.RemoveAll(building.path)

	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: filepath.Dir(work.path), Args: []string{"init", "--quiet", building.path},
	}); err != nil {
		return Result{}, err
	}
	history := "tips"
	if work.full {
		history = "full"
	}
	if err := workspaces.runner.SetConfig(ctx, building.path,
		"remote.origin.url", workspaces.pools.Address(work.repository),
		"gitd.url", work.repository.HTTPS(),
		"gitd.pool", workspaces.pools.Path(work.repository),
		"gitd.history", history,
		"gitd.strategy", string(strategy.Copy),
	); err != nil {
		return Result{}, err
	}
	result, used, err := workspaces.bringUp(ctx, building)
	result.Strategy = strategy.Copy.Proto()
	if err != nil {
		return Result{}, err
	}
	if err := os.Remove(work.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Result{}, err
	}
	if err := os.Rename(building.path, work.path); err != nil {
		return Result{}, err
	}
	if err := workspaces.claim(ctx, work.path, work.repository); err != nil {
		return Result{}, err
	}
	return result, workspaces.settled(ctx, work, result, used)
}
