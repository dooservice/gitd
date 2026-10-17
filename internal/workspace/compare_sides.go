package workspace

import (
	"context"
	"os"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// side is one work being compared: its repository, the commit its own changes are read
// against, the paths it changed since — none for a branch, which is its commit — and how
// its files are read: through the workspace's root, or from the pool.
type side struct {
	name       string
	repository repo.Repo
	base       string
	changed    []string
	root       *os.Root
	entries    map[string]pool.TreeEntry
}

// openSide opens one side of a comparison; url is the repository of a branch side. close
// releases what it holds.
func (workspaces *Workspaces) openSide(
	ctx context.Context, asked *gitdv1.Side, url string, includeIgnored bool,
) (*side, error) {
	switch {
	case asked.GetWorkspace() != "":
		set, err := workspaces.changeSet(ctx, asked.GetWorkspace(), includeIgnored)
		if err != nil {
			return nil, err
		}
		root, err := os.OpenRoot(set.path)
		if err != nil {
			return nil, err
		}
		opened := &side{name: set.path, repository: set.repository, base: set.base, root: root}
		for _, file := range set.files {
			opened.changed = append(opened.changed, file.path)
		}
		return opened, nil
	case asked.GetBranch() != "":
		repository, named := repo.Of(url)
		if !named {
			return nil, git.Unnamed(url)
		}
		branch := asked.GetBranch()
		if err := workspaces.validBranch(ctx, branch); err != nil {
			return nil, err
		}
		dir := workspaces.pools.Path(repository)
		for _, ref := range []string{"refs/gitd/published/" + branch, "refs/heads/" + branch} {
			commit, present, err := workspaces.runner.Resolve(ctx, dir, ref)
			if err != nil {
				return nil, git.About(err, repository.HTTPS())
			}
			if present {
				return &side{name: branch, repository: repository, base: commit}, nil
			}
		}
		failure := git.Fail(git.NotFound, "the pool of %s holds no branch %q", repository.HTTPS(), branch)
		failure.Repository = repository.HTTPS()
		return nil, failure
	default:
		return nil, git.Fail(git.BadAddress, "a side is a workspace or a branch")
	}
}

// load reads what the side's base commit has at or under paths, for a branch side.
func (workspaces *Workspaces) load(ctx context.Context, opened *side, paths []string) error {
	if opened.root != nil {
		return nil
	}
	entries, err := workspaces.pools.TreeEntries(ctx, opened.repository, opened.base, paths)
	opened.entries = entries
	return err
}

// object is a file as the side has it — its object and mode as git records them — and false
// when it has none there.
func (opened *side) object(path, format string) (string, string, bool, error) {
	if opened.root != nil {
		return workspaceObject(opened.root, path, format)
	}
	entry, present := opened.entries[path]
	return entry.Object, entry.Mode, present, nil
}

// expand replaces each path the side holds as a directory with the files under it.
func (opened *side) expand(paths []string) ([]string, error) {
	if opened.root != nil {
		return expandDirectories(opened.root, paths)
	}
	expanded := append([]string(nil), paths...)
	for path := range opened.entries {
		expanded = append(expanded, path)
	}
	return expanded, nil
}

// isDirectory says whether the side holds path as a directory rather than a file.
func (opened *side) isDirectory(path string) bool {
	if opened.root != nil {
		info, err := opened.root.Lstat(path)
		return err == nil && info.IsDir()
	}
	for entry := range opened.entries {
		if strings.HasPrefix(entry, path+"/") {
			return true
		}
	}
	return false
}

func (opened *side) close() {
	if opened.root != nil {
		_ = opened.root.Close()
	}
}
