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
// are resolved: a link must not carry a workspace out of where gitd may write. Nor may it lie
// inside another workspace, whose directories whoever works there can change between gitd's
// check and its write.
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
		// A root reached through a link — /tmp and /var are links on macOS — is compared as
		// it resolves, as the path is.
		if real, err := filepath.EvalSymlinks(root); err == nil && !inside(resolved, root) {
			root = real
		}
		if !inside(resolved, root) {
			continue
		}
		if holder := enclosingWorkspace(root, resolved); holder != "" {
			return "", git.Fail(git.PathRefused, "%s is inside the workspace %s", path, holder)
		}
		return path, nil
	}
	return "", git.Fail(git.PathRefused, "%s is outside the directories gitd may write in", path)
}

// enclosingWorkspace is the directory between root and path, path excluded, that holds a
// `.git`: a workspace path lies in. Empty when there is none.
func enclosingWorkspace(root, path string) string {
	for parent := filepath.Dir(path); inside(parent, root); parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return parent
		}
	}
	return ""
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
