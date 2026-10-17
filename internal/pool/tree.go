package pool

import (
	"context"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// pathsPerCall is at most how many paths one git command is given, well under any
// system's limit on a command line.
const pathsPerCall = 500

// TreeEntry is a file of a commit: its mode and its object.
type TreeEntry struct {
	Mode   string
	Object string
}

// TreeEntries are the files of a commit at or under the given paths, by path.
func (pools *Pools) TreeEntries(ctx context.Context, repository repo.Repo, commit string, paths []string) (map[string]TreeEntry, error) {
	entries := map[string]TreeEntry{}
	for start := 0; start < len(paths); start += pathsPerCall {
		chunk := paths[start:min(start+pathsPerCall, len(paths))]
		args := append([]string{"ls-tree", "-r", "-z", "--full-tree", "--end-of-options", commit, "--"}, chunk...)
		out, err := pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Args: args})
		if err != nil {
			return nil, git.About(err, repository.HTTPS())
		}
		for line := range strings.SplitSeq(out, "\x00") {
			meta, path, found := strings.Cut(line, "\t")
			fields := strings.Fields(meta)
			if !found || len(fields) != 3 {
				continue
			}
			entries[path] = TreeEntry{Mode: fields[0], Object: fields[2]}
		}
	}
	return entries, nil
}

// Submodules are the paths a commit's .gitmodules names, read in the pool.
func (pools *Pools) Submodules(ctx context.Context, repository repo.Repo, commit string) ([]string, error) {
	dir := pools.Path(repository)
	if _, present, err := pools.runner.ResolveObject(ctx, dir, commit+":.gitmodules"); err != nil || !present {
		return nil, err
	}
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Differences: true, Args: []string{
		"config", "-z", "--blob", commit + ":.gitmodules", "--get-regexp", `^submodule\..*\.path$`,
	}})
	if err != nil {
		return nil, git.About(err, repository.HTTPS())
	}
	var paths []string
	for entry := range strings.SplitSeq(out, "\x00") {
		if _, path, found := strings.Cut(entry, "\n"); found && path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// ObjectFormat is the hash a repository names its objects with: sha1 or sha256.
func (pools *Pools) ObjectFormat(ctx context.Context, repository repo.Repo) (string, error) {
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Args: []string{"rev-parse", "--show-object-format"}})
	return strings.TrimSpace(out), err
}

// Blob is the content of an object of a repository.
func (pools *Pools) Blob(ctx context.Context, repository repo.Repo, object string) (string, error) {
	return pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Args: []string{"cat-file", "blob", object}})
}
