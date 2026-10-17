package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/template"
)

func TestATemplateIsAWorkspaceDetachedAtItsCommitSharedAndForgottenByItsPool(t *testing.T) {
	root := t.TempDir()
	upstream, pools := filepath.Join(root, "upstream"), filepath.Join(root, "pools")
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	run(t, root, "init", "-q", "-b", "main", upstream)
	if err := os.WriteFile(filepath.Join(upstream, "README.md"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, upstream, "add", "README.md")
	run(t, upstream, "commit", "-q", "-m", "first")
	first := run(t, upstream, "rev-parse", "HEAD")
	run(t, upstream, "commit", "-q", "--allow-empty", "-m", "second")
	run(t, root, "clone", "-q", "--bare", upstream, repository.Dir(pools))

	runner := git.NewRunner("git")
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := credentials.NewKeys(filepath.Join(root, "state"), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pooled := pool.New(pools, runner, keys, locks, nil, 0)
	workspaces := &Workspaces{runner: runner, pools: pooled}
	store := template.New(filepath.Join(root, "templates"), locks, workspaces)
	ctx := context.Background()
	key := template.Key{Repository: repository, Commit: first}

	dir, err := store.Take(ctx, key, filepath.Join(root, "agent-1"))
	if err != nil {
		t.Fatal(err)
	}
	if at := run(t, dir, "rev-parse", "HEAD"); at != first {
		t.Errorf("the template is at %s, not %s", at, first)
	}
	if branch := run(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); branch != "HEAD" {
		t.Errorf("the template is on %s, not detached", branch)
	}
	for setting, want := range map[string]string{"core.checkStat": "minimal", "core.trustctime": "false"} {
		if got := run(t, dir, "config", setting); got != want {
			t.Errorf("%s = %s; want %s", setting, got, want)
		}
	}
	if !slices.Contains(pool.Workspaces(repository.Dir(pools)), dir) {
		t.Errorf("the pool does not know its template: %v", pool.Workspaces(repository.Dir(pools)))
	}

	before, _ := os.Stat(filepath.Join(dir, "README.md"))
	if again, err := store.Take(ctx, key, filepath.Join(root, "agent-2")); err != nil || again != dir {
		t.Fatalf("second Take = %q, %v", again, err)
	}
	after, _ := os.Stat(filepath.Join(dir, "README.md"))
	if !os.SameFile(before, after) {
		t.Error("the template was built again for its second workspace")
	}

	if store.Retire(ctx, 0) != 1 {
		t.Fatal("an unused template was kept")
	}
	if slices.Contains(pool.Workspaces(repository.Dir(pools)), dir) {
		t.Error("the pool still counts a retired template")
	}
}
