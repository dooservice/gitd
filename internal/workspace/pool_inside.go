package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// A workspace mounted where its pool is not — into a Kubernetes pod by the CSI driver —
// names its pool by a path inside itself: its alternates say PoolInside, git reads it
// relative to the objects directory, `.git/gitd-pool`, and whoever mounts the workspace
// mounts the pool's objects there, read-only. gitd accepts it only while `.git/gitd-pool` is
// truly that directory, mounted: a plain directory, never a link, the very same directory as
// the objects of a pool lending to the workspace. Copying the objects there would not do.

// PoolInside is the alternates line of a workspace that carries its pool inside itself.
const PoolInside = "../" + poolMount

// poolMount is where, in a workspace's git directory, its pool's objects are mounted.
const poolMount = "gitd-pool"

// carryPool makes the workspace at path name its pool from inside itself, and answers the
// pool's objects directory, to mount at `.git/gitd-pool`.
func (workspaces *Workspaces) carryPool(ctx context.Context, path string, repository repo.Repo) (string, error) {
	gitDir := filepath.Join(path, ".git")
	dir, err := openGitDir(gitDir)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	if err := dir.MkdirAll(poolMount, 0o755); err != nil {
		return "", err
	}
	if err := workspaces.runner.SetConfig(ctx, path, "gitd.poolinside", "true"); err != nil {
		return "", err
	}
	if err := writeInside(dir, filepath.Join("objects", "info", "alternates"), []byte(PoolInside+"\n")); err != nil {
		return "", err
	}
	return filepath.Join(workspaces.pools.Path(repository), "objects"), nil
}

// carriesPool says whether the git directory at gitDir names its pool from inside itself,
// as its configuration says.
func (workspaces *Workspaces) carriesPool(gitDir string) bool {
	said, _ := workspaces.runner.Config(context.Background(), filepath.Join(gitDir, "config"), "gitd.poolinside")
	return strings.TrimSpace(said) == "true"
}

// mountedPool says whether the workspace's `.git/gitd-pool` is the objects directory of a
// pool lending to it, mounted there.
func (workspaces *Workspaces) mountedPool(workspace, gitDir string) bool {
	mounted := filepath.Join(gitDir, poolMount)
	info, err := os.Lstat(mounted)
	if err != nil || !info.IsDir() {
		return false
	}
	for _, dir := range workspaces.pools.All() {
		if !lends(dir, workspace) {
			continue
		}
		objects, err := os.Stat(filepath.Join(dir, "objects"))
		if err == nil && os.SameFile(info, objects) {
			return true
		}
	}
	return false
}

func lends(dir, workspace string) bool {
	for _, borrower := range pool.Workspaces(dir) {
		if borrower == workspace {
			return true
		}
	}
	return false
}
