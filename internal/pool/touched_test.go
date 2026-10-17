package pool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/repo"
)

// gitIn runs git in dir as a test author.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	command.Dir = dir
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheCommitsToPublishSayWhichPathsTheyTouch(t *testing.T) {
	root := t.TempDir()
	upstream, work := filepath.Join(root, "upstream"), filepath.Join(root, "work")
	gitIn(t, root, "init", "-q", "-b", "main", upstream)
	write(t, filepath.Join(upstream, ".github/workflows/ci.yml"), "ci\n")
	write(t, filepath.Join(upstream, "tests/a_test.go"), "t\n")
	write(t, filepath.Join(upstream, "app.go"), "a\n")
	gitIn(t, upstream, "add", "-A")
	gitIn(t, upstream, "commit", "-q", "-m", "base")
	gitIn(t, root, "clone", "-q", upstream, work)

	// People change the CI on main while the agent works.
	write(t, filepath.Join(upstream, ".github/workflows/ci.yml"), "ci 2\n")
	gitIn(t, upstream, "commit", "-q", "-am", "people change the CI")
	// The agent changes app.go, renames a test away and merges main.
	write(t, filepath.Join(work, "app.go"), "b\n")
	gitIn(t, work, "mv", "tests/a_test.go", "moved.go")
	gitIn(t, work, "commit", "-q", "-am", "agent")
	gitIn(t, work, "pull", "-q", "--no-rebase", "--no-edit", "origin", "main")

	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	pools := filepath.Join(root, "pools")
	gitIn(t, root, "clone", "-q", "--bare", upstream, repository.Dir(pools))
	locks, _ := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	keys, _ := credentials.NewKeys(filepath.Join(root, "state"), nil, nil, nil)
	kept := New(pools, git.NewRunner("git"), keys, locks, nil, 0)
	ctx := context.Background()
	if _, _, err := kept.Stage(ctx, repository, filepath.Join(work, ".git"), "agents/fix"); err != nil {
		t.Fatal(err)
	}

	all, err := kept.Touched(ctx, repository, "agents/fix", Paths{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(all, []string{"app.go", "moved.go", "tests/a_test.go"}) {
		t.Errorf("touched = %v; the CI people changed on main must not be the agent's", all)
	}
	protected, _ := kept.Touched(ctx, repository, "agents/fix", Paths{Among: []string{".github/workflows/**"}})
	if len(protected) != 0 {
		t.Errorf("protected touched = %v", protected)
	}
	deleted, _ := kept.Touched(ctx, repository, "agents/fix", Paths{Among: []string{"**/*_test.*"}, Deleted: true})
	if !slices.Equal(deleted, []string{"tests/a_test.go"}) {
		t.Errorf("deleted = %v; a test renamed away is deleted", deleted)
	}
	allowed, _ := kept.Touched(ctx, repository, "agents/fix", Paths{Among: []string{"**/*.go"}, Except: []string{"tests/**"}})
	if !slices.Equal(allowed, []string{"app.go", "moved.go"}) {
		t.Errorf("among Go files except tests = %v", allowed)
	}
	added, removed, err := kept.Lines(ctx, repository, "agents/fix")
	if err != nil || added != 2 || removed != 2 {
		t.Errorf("lines = +%d -%d, %v; want app.go's change and the rename, not main's", added, removed, err)
	}
}
