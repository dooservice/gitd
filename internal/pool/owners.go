package pool

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dooservice/gitd/internal/repo"
)

// Besides the workspaces that borrow from it, each pool keeps, apart, those it is the own
// repository of: not those that borrow it only for a submodule. A workspace names its
// repository in its own configuration, which whoever works in it can write; this list, which
// only gitd writes, is what that name is checked against, so a workspace cannot pass for one
// of another repository and be published there with gitd's credential.

// Own notes that the workspace at path is one of the repository's own.
func (pools *Pools) Own(ctx context.Context, repository repo.Repo, path string) error {
	return pools.editList(ctx, repository, ownersOf, func(paths []string) []string {
		if slices.Contains(paths, path) {
			return paths
		}
		return append(paths, path)
	})
}

// Disown notes that the workspace at path is no longer one of the repository's own.
func (pools *Pools) Disown(ctx context.Context, repository repo.Repo, path string) error {
	return pools.editList(ctx, repository, ownersOf, func(paths []string) []string {
		return slices.DeleteFunc(paths, func(noted string) bool { return noted == path })
	})
}

// Owns says whether the workspace at path is one of the repository's own.
func (pools *Pools) Owns(repository repo.Repo, path string) bool {
	return slices.Contains(readList(ownersOf(pools.Path(repository))), path)
}

// HasOwners says whether the pool at dir keeps its list of own workspaces yet: a pool made
// before gitd kept one has none, until RecordOwners writes it.
func HasOwners(dir string) bool {
	_, err := os.Stat(ownersOf(dir))
	return err == nil
}

// RecordOwners writes the list of own workspaces of the pool at dir.
func RecordOwners(dir string, paths []string) error {
	return WriteAtomic(ownersOf(dir), []byte(strings.Join(paths, "\n")+"\n"))
}

func ownersOf(dir string) string {
	return filepath.Join(dir, "gitd-owned")
}
