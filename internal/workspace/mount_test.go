package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

func TestASubmoduleThatMovedIsMountedWithoutTouchingTheSuperproject(t *testing.T) {
	root := t.TempDir()
	sub, work, pools := filepath.Join(root, "sub"), filepath.Join(root, "work"), filepath.Join(root, "pools")
	subRepo := repo.Repo{Host: "github.com", Path: "acme/sub"}
	run(t, root, "init", "-q", "-b", "main", sub)
	run(t, sub, "commit", "-q", "--allow-empty", "-m", "a")
	commit := run(t, sub, "rev-parse", "HEAD")
	run(t, root, "clone", "-q", "--bare", sub, subRepo.Dir(pools))

	// The submodule's git directory is there from before, but its checkout is not: it
	// moved from `old/sub` to `new/sub`.
	run(t, root, "init", "-q", "-b", "main", work)
	run(t, work, "remote", "add", "origin", "https://github.com/acme/super")
	if err := os.MkdirAll(filepath.Join(work, ".git", "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, work, "init", "-q", "--separate-git-dir", filepath.Join(work, ".git", "modules", "sub"), filepath.Join(work, "old", "sub"))
	if err := os.RemoveAll(filepath.Join(work, "old")); err != nil {
		t.Fatal(err)
	}

	runner := git.NewRunner("git")
	workspaces := &Workspaces{runner: runner, pools: pool.New(pools, runner, nil, nil, nil, 0)}
	job := job{path: work, repository: repo.Repo{Host: "github.com", Path: "acme/super"}}
	moved := submodule{name: "sub", path: "new/sub", url: "https://github.com/acme/sub"}
	if err := workspaces.mount(context.Background(), job, moved, subRepo, commit); err != nil {
		t.Fatal(err)
	}

	if origin := run(t, work, "config", "remote.origin.url"); origin != "https://github.com/acme/super" {
		t.Errorf("the superproject's origin became %s", origin)
	}
	if at := run(t, filepath.Join(work, "new", "sub"), "rev-parse", "HEAD"); at != commit {
		t.Errorf("the submodule is at %s, not %s", at, commit)
	}
	if worktree := run(t, filepath.Join(work, "new", "sub"), "config", "core.worktree"); worktree != "../../../new/sub" {
		t.Errorf("core.worktree = %s", worktree)
	}
}
