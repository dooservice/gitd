package pool

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/repo"
)

// Use notes that the workspace at path borrows from the pool of a repository.
//
// Each pool keeps the paths of the workspaces that borrow from it, and the time of that
// file is when the pool was last used. Both are what keeps a pool from being retired
// while something still needs it.
func (pools *Pools) Use(ctx context.Context, repository repo.Repo, path string) error {
	return pools.editRegistry(ctx, repository, func(paths []string) []string {
		if slices.Contains(paths, path) {
			return paths
		}
		return append(paths, path)
	})
}

// Forget notes that the workspace at path no longer borrows from the pool of a repository.
func (pools *Pools) Forget(ctx context.Context, repository repo.Repo, path string) error {
	return pools.editRegistry(ctx, repository, func(paths []string) []string {
		return slices.DeleteFunc(paths, func(noted string) bool { return noted == path })
	})
}

func (pools *Pools) editRegistry(ctx context.Context, repository repo.Repo, change func([]string) []string) error {
	return pools.editList(ctx, repository, registryOf, change)
}

// editList changes, under the pool's lock, one of the lists of workspace paths it keeps.
func (pools *Pools) editList(
	ctx context.Context, repository repo.Repo, file func(string) string, change func([]string) []string,
) error {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return err
	}
	defer release()
	dir := pools.Path(repository)
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	paths := change(readList(file(dir)))
	return WriteAtomic(file(dir), []byte(strings.Join(paths, "\n")+"\n"))
}

func (pools *Pools) touch(dir string) {
	now := time.Now()
	if err := os.Chtimes(registryOf(dir), now, now); errors.Is(err, fs.ErrNotExist) {
		_ = WriteAtomic(registryOf(dir), nil)
	}
}

// Workspaces are the paths noted as borrowing from the pool at dir.
func Workspaces(dir string) []string {
	return readList(registryOf(dir))
}

// readList reads a list of workspace paths, one per line; a list not there is empty.
func readList(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var paths []string
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		if line := strings.TrimSpace(lines.Text()); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// LastUsed is when the pool at dir was last synced or borrowed from.
func LastUsed(dir string) time.Time {
	info, err := os.Stat(registryOf(dir))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func registryOf(dir string) string {
	return filepath.Join(dir, "gitd-workspaces")
}
