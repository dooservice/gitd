package workspace

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
)

// Besides its configuration, a git directory holds files git follows to other places: a
// `commondir` naming another git directory, `objects/info/alternates` naming other object
// stores, and any of its parts made a link. Whoever works in the workspace can write them,
// so gitd checks them before running git there: nothing may lead out of the workspace but
// the alternates gitd wrote, which name pools that lend to it.

// gitDirFiles are the parts of a git directory that must be plain directories or files,
// never links: what git reads objects, references and HEAD from.
var gitDirFiles = []struct {
	name string
	dir  bool
}{
	{"objects", true}, {"objects/info", true}, {"refs", true}, {"HEAD", false},
}

// checkGitDir refuses a git directory of the workspace whose parts lead out of it: a link,
// a `commondir`, or alternates naming anything but a pool that lends to the workspace.
func (workspaces *Workspaces) checkGitDir(workspace, gitDir string) error {
	if _, err := os.Lstat(filepath.Join(gitDir, "commondir")); !errors.Is(err, fs.ErrNotExist) {
		return git.Fail(git.ConfigRefused, "%s names another git directory (commondir)", gitDir)
	}
	for _, part := range gitDirFiles {
		info, err := os.Lstat(filepath.Join(gitDir, part.name))
		if errors.Is(err, fs.ErrNotExist) && part.name == "objects/info" {
			continue
		}
		if err != nil || info.IsDir() != part.dir || !info.IsDir() && !info.Mode().IsRegular() {
			return git.Fail(git.ConfigRefused, "%s/%s is not a plain %s", gitDir, part.name, kind(part.dir))
		}
	}
	return workspaces.checkAlternates(workspace, filepath.Join(gitDir, "objects", "info", "alternates"))
}

// checkAlternates refuses alternates that name anything but the objects of a pool lending to
// the workspace.
func (workspaces *Workspaces) checkAlternates(workspace, alternates string) error {
	info, err := os.Lstat(alternates)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return git.Fail(git.ConfigRefused, "%s is not a plain file", alternates)
	}
	file, err := os.Open(alternates)
	if err != nil {
		return err
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" {
			continue
		}
		if !workspaces.lendingPool(workspace, line) {
			return git.Fail(git.ConfigRefused, "%s names %s, which is not a pool lending to the workspace", alternates, line)
		}
	}
	return lines.Err()
}

// lendingPool says whether objects is the objects directory of a pool that lists the
// workspace among those borrowing from it.
func (workspaces *Workspaces) lendingPool(workspace, objects string) bool {
	if filepath.Base(objects) != "objects" || filepath.Clean(objects) != objects {
		return false
	}
	dir := filepath.Dir(objects)
	return inside(dir, workspaces.pools.Dir) && slices.Contains(pool.Workspaces(dir), workspace)
}

func kind(dir bool) string {
	if dir {
		return "directory"
	}
	return "file"
}
