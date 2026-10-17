package workspace

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/git"
)

// Whoever works in a workspace can write its `.git`, and with it the configuration git reads
// there: a filter, a hook path or an alternative command would then run as gitd's user the
// next time gitd runs git in the workspace. So before it does, gitd checks every key of the
// workspace's configuration, and of each submodule's, against what it is willing to run
// under: the keys it writes itself, and a few that only describe the repository or its
// author (config_keys.go). Anything else is refused, naming the keys, rather than run or
// quietly removed.

// guard refuses a workspace gitd would not safely run git in: one whose `.git` is not a
// directory of its own, or whose configuration, or a submodule's, sets a key outside
// allowedKeys.
func (workspaces *Workspaces) guard(ctx context.Context, path string) error {
	gitDir := filepath.Join(path, ".git")
	if info, err := os.Lstat(gitDir); err != nil || !info.IsDir() {
		return git.Fail(git.ConfigRefused, "%s/.git is not a directory of its own", path)
	}
	if err := workspaces.checkConfig(ctx, path, filepath.Join(gitDir, "config"), false); err != nil {
		return err
	}
	modules := filepath.Join(gitDir, "modules")
	return filepath.WalkDir(modules, func(file string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			if file == modules && os.IsNotExist(err) {
				return nil
			}
			return err
		case entry.Type()&fs.ModeSymlink != 0:
			return git.Fail(git.ConfigRefused, "%s is a symbolic link inside .git/modules", file)
		case entry.Name() == "config" && entry.Type().IsRegular():
			return workspaces.checkConfig(ctx, path, file, true)
		}
		return nil
	})
}

// checkConfig refuses a configuration file that is not a plain file, or that sets a key
// outside allowedKeys; a submodule's may also point at its checkout inside the workspace.
func (workspaces *Workspaces) checkConfig(ctx context.Context, workspace, file string, module bool) error {
	if info, err := os.Lstat(file); err != nil || !info.Mode().IsRegular() {
		return git.Fail(git.ConfigRefused, "%s is not a plain file", file)
	}
	entries, err := workspaces.runner.ConfigEntries(ctx, file)
	if err != nil {
		return err
	}
	var refused []string
	for _, entry := range entries {
		switch {
		case allowedKeys.MatchString(entry.Key):
		case module && entry.Key == moduleWorkTree && insideCheckout(workspace, filepath.Dir(file), entry.Value):
		default:
			refused = append(refused, entry.Key)
		}
	}
	if len(refused) > 0 {
		return git.Fail(git.ConfigRefused,
			"%s sets %s, which gitd does not run git under: unset them (git config --unset) and try again",
			file, strings.Join(refused, ", "))
	}
	return nil
}

// insideCheckout is whether a submodule's core.worktree, relative to its git directory,
// names a directory of the workspace that is not inside its `.git`.
func insideCheckout(workspace, gitDir, workTree string) bool {
	if filepath.IsAbs(workTree) {
		return false
	}
	target := filepath.Join(gitDir, workTree)
	return inside(target, workspace) && !inside(target, filepath.Join(workspace, ".git")) &&
		target != filepath.Join(workspace, ".git")
}
