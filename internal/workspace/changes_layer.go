package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
)

// layerCandidates are the paths that may have changed in an overlay workspace: everything
// in its own layer outside .git — a file written there, a mark of a deletion, a directory
// that may hide what its lower layers hold — what the warm layer it stands on changes,
// and what differs between its template's commit and the commit gitd last put it on,
// which updating or checking it out brought. It also answers that commit, its template's
// when gitd noted none. What the warm layer holds that git ignores is the layer's, not
// the workspace's, and is never walked.
func (workspaces *Workspaces) layerCandidates(
	ctx context.Context, repository repo.Repo, workspace string, record strategy.Record, upper string, root *os.Root,
) ([]string, string, error) {
	lower := record.Template
	templateCommit, _, err := workspaces.runner.Resolve(ctx, lower, "HEAD")
	if err != nil {
		return nil, "", err
	}
	base, noted := workspaces.pools.Base(repository, workspace)
	if !noted {
		base = templateCommit
	}
	candidates, err := layerPaths(record.Lowers(), upper, root)
	if err != nil {
		return nil, "", err
	}
	if record.Warm != "" {
		warm, err := workspaces.templates.WarmAt(record.Warm)
		if err != nil {
			return nil, "", err
		}
		candidates = append(candidates, warm.Changed...)
	}
	if base != templateCommit {
		moved, err := workspaces.pools.ChangedBetween(ctx, repository, templateCommit, base)
		if err != nil {
			return nil, "", err
		}
		candidates = append(candidates, moved...)
	}
	return candidates, base, nil
}

// layerPaths walks a workspace's own layer, never following a link. A file or a link there
// is a candidate; so is a mark of a deletion (a character device numbered 0), which is
// where a lower layer's file or directory was. A directory of the layer a lower layer also
// has may hide what that layer holds under it — overlay marks such a directory in an
// attribute gitd, unprivileged, cannot read — so what a lower layer has there and the
// workspace does not show is a candidate too.
func layerPaths(lowers []string, upper string, root *os.Root) ([]string, error) {
	var candidates []string
	err := filepath.WalkDir(upper, func(walked string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(upper, walked)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".git" || strings.HasPrefix(relative, ".git/") {
			return fs.SkipDir
		}
		switch {
		case entry.IsDir():
			for _, lower := range lowers {
				hidden, err := hiddenUnder(lower, relative, root)
				if err != nil {
					return err
				}
				candidates = append(candidates, hidden...)
			}
		case entry.Type()&fs.ModeCharDevice != 0 && isWhiteout(walked):
			candidates = append(candidates, relative)
		case entry.Type().IsRegular() || entry.Type()&fs.ModeSymlink != 0:
			candidates = append(candidates, relative)
		}
		return nil
	})
	return candidates, err
}

// isWhiteout says whether a character device is overlay's mark of a deletion.
func isWhiteout(file string) bool {
	info, err := os.Lstat(file)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Rdev == 0
}

// hiddenUnder lists what the template has directly under directory and the workspace no
// longer shows there.
func hiddenUnder(lower, directory string, root *os.Root) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(lower, filepath.FromSlash(directory)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hidden []string
	for _, entry := range entries {
		name := path.Join(directory, entry.Name())
		if _, err := root.Lstat(filepath.FromSlash(name)); errors.Is(err, fs.ErrNotExist) {
			hidden = append(hidden, name)
		}
	}
	return hidden, nil
}
