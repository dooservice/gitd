package pool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/repo"
)

func TestTheCommitAWorkspaceWasPutOnIsKeptAndReplaced(t *testing.T) {
	root := t.TempDir()
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	pools := filepath.Join(root, "pools")
	gitIn(t, root, "init", "-q", "--bare", repository.Dir(pools))
	locks, _ := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	keys, _ := credentials.NewKeys(filepath.Join(root, "state"), nil, nil, nil)
	kept := New(pools, git.NewRunner("git"), keys, locks, nil, 0)
	ctx := context.Background()
	path := "/var/lib/gitd/workspaces/agent one"
	if _, found := kept.Base(repository, path); found {
		t.Fatal("a base before any was noted")
	}
	for _, commit := range []string{"aaaa", "bbbb"} {
		if err := kept.SetBase(ctx, repository, path, commit); err != nil {
			t.Fatal(err)
		}
	}
	if err := kept.SetBase(ctx, repository, "/var/lib/gitd/workspaces/agent-2", "cccc"); err != nil {
		t.Fatal(err)
	}
	if commit, found := kept.Base(repository, path); !found || commit != "bbbb" {
		t.Errorf("base = %q, %v; want the last one noted", commit, found)
	}
	if err := kept.ForgetBase(ctx, repository, path); err != nil {
		t.Fatal(err)
	}
	if _, found := kept.Base(repository, path); found {
		t.Error("a forgotten base is still there")
	}
	if commit, _ := kept.Base(repository, "/var/lib/gitd/workspaces/agent-2"); commit != "cccc" {
		t.Errorf("another workspace's base = %q", commit)
	}
}
