package pool

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dooservice/gitd/internal/repo"
)

// Each pool keeps, apart from its workspaces, which branches each of its own workspaces
// published: a publish of such a branch again from the same workspace moves it forward
// without asking. Only gitd writes this list; the workspace's own configuration, which
// whoever works in it can write, is never what says a branch may be moved.

// publishedEntry is one line of the list: the branch, a tab, the workspace's path. A branch
// name holds no tab, so the first one ends it.
type publishedEntry struct {
	branch string
	path   string
}

func (entry publishedEntry) String() string {
	return entry.branch + "\t" + entry.path
}

func parsePublished(line string) (publishedEntry, bool) {
	branch, path, ok := strings.Cut(line, "\t")
	return publishedEntry{branch: branch, path: path}, ok && branch != "" && path != ""
}

// NotePublished notes that the workspace at path published branch.
func (pools *Pools) NotePublished(ctx context.Context, repository repo.Repo, path, branch string) error {
	line := publishedEntry{branch: branch, path: path}.String()
	return pools.editList(ctx, repository, publishedOf, func(lines []string) []string {
		if slices.Contains(lines, line) {
			return lines
		}
		return append(lines, line)
	})
}

// Published is the branches the workspace at path published, as noted.
func (pools *Pools) Published(repository repo.Repo, path string) []string {
	var branches []string
	for _, line := range readList(publishedOf(pools.Path(repository))) {
		if entry, ok := parsePublished(line); ok && entry.path == path {
			branches = append(branches, entry.branch)
		}
	}
	return branches
}

// ForgetPublished forgets what the workspace at path published, as it goes.
func (pools *Pools) ForgetPublished(ctx context.Context, repository repo.Repo, path string) error {
	return pools.editList(ctx, repository, publishedOf, func(lines []string) []string {
		return slices.DeleteFunc(lines, func(line string) bool {
			entry, ok := parsePublished(line)
			return ok && entry.path == path
		})
	})
}

func publishedOf(dir string) string {
	return filepath.Join(dir, "gitd-published")
}

// Owned is every workspace the pools under Dir own that is still on disk, once each.
func (pools *Pools) Owned() []string {
	var owned []string
	for _, dir := range pools.All() {
		for _, path := range readList(ownersOf(dir)) {
			if _, err := os.Stat(path); err == nil && !slices.Contains(owned, path) {
				owned = append(owned, path)
			}
		}
	}
	slices.Sort(owned)
	return owned
}
