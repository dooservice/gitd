package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/git"
)

// allowed is a path cleaned, when it lies inside one of the roots once its symbolic links
// are resolved: a link must not carry a workspace out of where gitd may write.
func (workspaces *Workspaces) allowed(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", git.Fail(git.PathRefused, "%q is not an absolute path", path)
	}
	path = filepath.Clean(path)
	existing := nearest(path)
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	resolved = filepath.Join(resolved, strings.TrimPrefix(path, existing))
	for _, root := range workspaces.roots {
		if inside(resolved, root) {
			return path, nil
		}
	}
	return "", git.Fail(git.PathRefused, "%s is outside the directories gitd may write in", path)
}

// nearest is the deepest part of a path that exists, the path itself included.
func nearest(path string) string {
	for {
		if _, err := os.Stat(path); err == nil || path == filepath.Dir(path) {
			return path
		}
		path = filepath.Dir(path)
	}
}

// inside is whether a path lies under root, and is not root itself.
func inside(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	climbs := relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
	return err == nil && relative != "." && !climbs
}

// vacant is a path where nothing is, or an empty directory.
func vacant(path string) bool {
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		_, err = os.Lstat(path)
		return errors.Is(err, fs.ErrNotExist)
	}
	return err == nil && len(entries) == 0
}
