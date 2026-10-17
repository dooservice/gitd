package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
)

// openGitDir opens a git directory of a workspace as an os.Root, refusing one that is itself
// a link: every write gitd makes in it then stays in it, whatever link the sandbox left
// inside.
func openGitDir(gitDir string) (*os.Root, error) {
	if info, err := os.Lstat(gitDir); err != nil || !info.IsDir() {
		return nil, git.Fail(git.ConfigRefused, "%s is not a directory of its own", gitDir)
	}
	return os.OpenRoot(gitDir)
}

// writeInside writes data to name within dir, atomically: to a temporary file beside it,
// then renamed into place, both through dir.
func writeInside(dir *os.Root, name string, data []byte) error {
	if err := dir.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return git.Fail(git.ConfigRefused, "%s: %v", name, err)
	}
	temp := pool.Temp(name)
	if err := dir.WriteFile(temp, data, 0o644); err != nil {
		return git.Fail(git.ConfigRefused, "%s: %v", name, err)
	}
	if err := dir.Rename(temp, name); err != nil {
		_ = dir.Remove(temp)
		return git.Fail(git.ConfigRefused, "%s: %v", name, err)
	}
	return nil
}

// removeInside removes name within dir; one not there is removed already.
func removeInside(dir *os.Root, name string) error {
	if err := dir.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
