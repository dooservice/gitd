package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// link adds the pool's objects to the alternates of the git directory gitDir, keeping any
// it already had, and undoes it when `fsck`, run in the checkout at workTree, finds the
// clone does not read whole that way.
func (workspaces *Workspaces) link(
	ctx context.Context, workTree, gitDir string, repository repo.Repo, report Report,
) error {
	dir, err := openGitDir(gitDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	alternatesIn := filepath.Join("objects", "info", "alternates")
	previous, err := dir.ReadFile(alternatesIn)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	linked := strings.TrimRight(string(previous), "\n")
	if linked != "" {
		linked += "\n"
	}
	linked += filepath.Join(workspaces.pools.Path(repository), "objects") + "\n"
	if err := writeInside(dir, alternatesIn, []byte(linked)); err != nil {
		return err
	}

	if report != nil {
		report(&gitdv1.Progress{Phase: "Checking connectivity", Percent: -1, Repository: repository.HTTPS()})
	}
	check := git.Cmd{Dir: workTree, Args: []string{"fsck", "--connectivity-only", "--no-progress"}}
	_, err = workspaces.runner.Run(ctx, check)
	if err == nil {
		return nil
	}
	unlink(dir, alternatesIn, previous)
	if failure, ok := git.AsFailure(err); ok && failure.Reason != git.Timeout {
		failure.Reason = git.Corrupt
		failure.Repository = repository.HTTPS()
	}
	return err
}

func unlink(dir *os.Root, alternates string, previous []byte) {
	if len(previous) == 0 {
		_ = removeInside(dir, alternates)
		return
	}
	_ = writeInside(dir, alternates, previous)
}

// size is what the files under dir take.
func size(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
