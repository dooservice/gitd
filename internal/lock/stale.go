package lock

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClearStale removes the `.lock` files git leaves behind when it is killed, from the git
// directory at gitDir, once they are older than grace.
//
// Only the files git is known to take, never any `*.lock`: something else may own one.
// It is called with the repository's lock held, so no git of gitd's is running there.
func ClearStale(gitDir string, grace time.Duration) {
	for _, name := range []string{
		"index.lock", "HEAD.lock", "ORIG_HEAD.lock", "FETCH_HEAD.lock",
		"packed-refs.lock", "config.lock", "shallow.lock",
	} {
		removeIfStale(filepath.Join(gitDir, name), grace)
	}
	_ = filepath.WalkDir(filepath.Join(gitDir, "refs"), func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".lock") {
			removeIfStale(path, grace)
		}
		return nil
	})
}

func removeIfStale(path string, grace time.Duration) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) < grace {
		return
	}
	if os.Remove(path) == nil {
		slog.Warn("stale git lock removed", "path", path)
	}
}
