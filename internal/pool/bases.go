package pool

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dooservice/gitd/internal/repo"
)

// Each pool keeps, for each of its own workspaces, the commit gitd last put it on — when it
// made it, updated it or checked it out — as `<commit> <path>` lines only gitd writes. What
// an agent changed is read against it: a workspace's own configuration, which the agent
// can write, never says.

// SetBase notes the commit gitd put the workspace at path on.
func (pools *Pools) SetBase(ctx context.Context, repository repo.Repo, path, commit string) error {
	return pools.editList(ctx, repository, basesOf, func(lines []string) []string {
		lines = slices.DeleteFunc(lines, func(line string) bool { return basePath(line) == path })
		return append(lines, commit+" "+path)
	})
}

// ForgetBase drops what was noted of the workspace at path.
func (pools *Pools) ForgetBase(ctx context.Context, repository repo.Repo, path string) error {
	return pools.editList(ctx, repository, basesOf, func(lines []string) []string {
		return slices.DeleteFunc(lines, func(line string) bool { return basePath(line) == path })
	})
}

// Base is the commit gitd last put the workspace at path on, if it noted one.
func (pools *Pools) Base(repository repo.Repo, path string) (string, bool) {
	for _, line := range readList(basesOf(pools.Path(repository))) {
		if commit, noted, found := strings.Cut(line, " "); found && noted == path {
			return commit, true
		}
	}
	return "", false
}

func basePath(line string) string {
	_, path, _ := strings.Cut(line, " ")
	return path
}

func basesOf(dir string) string {
	return filepath.Join(dir, "gitd-bases")
}
