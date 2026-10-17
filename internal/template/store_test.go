package template

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/repo"
)

// fakeBuilder writes one file as a checkout, counts its builds, and remembers what it was
// told to forget.
type fakeBuilder struct {
	builds    atomic.Int32
	forgotten []string
	fail      error
	mu        sync.Mutex
}

func (builder *fakeBuilder) BuildTemplate(_ context.Context, key Key, building, _ string) error {
	builder.builds.Add(1)
	if builder.fail != nil {
		if err := os.MkdirAll(building, 0o755); err != nil {
			return err
		}
		return builder.fail
	}
	time.Sleep(20 * time.Millisecond) // long enough for a racing Take to wait on the lock
	if err := os.MkdirAll(building, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(building, "README.md"), []byte(key.Commit), 0o644)
}

func (builder *fakeBuilder) ForgetTemplate(_ context.Context, _ Key, dir string) error {
	builder.mu.Lock()
	defer builder.mu.Unlock()
	builder.forgotten = append(builder.forgotten, dir)
	return nil
}

const commit = "4efeed310944064485f024fefa3ab3c8021be23d"

func newStore(t *testing.T, builder Builder) *Store {
	t.Helper()
	root := t.TempDir()
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return New(filepath.Join(root, "templates"), locks, builder)
}

func web() Key {
	return Key{Repository: repo.Repo{Host: "gitlab.com", Path: "acme/tools/web"}, Commit: commit}
}

func TestATemplateIsBuiltOnceAndSharedEvenWhenAskedForAtOnce(t *testing.T) {
	builder := &fakeBuilder{}
	store := newStore(t, builder)
	users := t.TempDir()

	var wait sync.WaitGroup
	dirs := make([]string, 5)
	for index := range dirs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			dir, err := store.Take(context.Background(), web(), filepath.Join(users, "agent", string(rune('a'+index))))
			if err != nil {
				t.Error(err)
			}
			dirs[index] = dir
		}()
	}
	wait.Wait()

	if got := builder.builds.Load(); got != 1 {
		t.Errorf("built %d times; want once", got)
	}
	for _, dir := range dirs {
		if dir != web().Dir(store.Dir) {
			t.Errorf("Take = %q; want %q", dir, web().Dir(store.Dir))
		}
	}
	if got := len(store.Users(web())); got != 5 {
		t.Errorf("%d users noted; want 5", got)
	}
	if got := web().Dir(store.Dir); got != filepath.Join(store.Dir, "gitlab.com", "acme", "tools", "web", commit) {
		t.Errorf("Dir = %q", got)
	}
}

func TestATemplateThatFailsToBuildLeavesNothingBehind(t *testing.T) {
	builder := &fakeBuilder{fail: errors.New("git failed")}
	store := newStore(t, builder)
	if _, err := store.Take(context.Background(), web(), "/agents/a"); err == nil {
		t.Fatal("a failed build was taken")
	}
	entries, _ := os.ReadDir(filepath.Dir(web().Dir(store.Dir)))
	if len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}

func TestOnlyAWholeCommitNamesATemplate(t *testing.T) {
	store := newStore(t, &fakeBuilder{})
	for _, commit := range []string{"", "main", "4efeed3", "../../etc", commit + "0"} {
		key := web()
		key.Commit = commit
		_, err := store.Take(context.Background(), key, "/agents/a")
		if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.BadAddress {
			t.Errorf("Take(%q) = %v; want BAD_ADDRESS", commit, err)
		}
	}
}

func TestATemplateIsRetiredOnlyWhenNobodyUsesItAndItHasRested(t *testing.T) {
	builder := &fakeBuilder{}
	store := newStore(t, builder)
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "agent-1")
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := store.Take(ctx, web(), user)
	if err != nil {
		t.Fatal(err)
	}

	if store.Retire(ctx, 0) != 0 || !exists(dir) {
		t.Fatal("retired while a workspace uses it")
	}
	if err := store.Let(ctx, web(), user); err != nil {
		t.Fatal(err)
	}
	if store.Retire(ctx, time.Hour) != 0 || !exists(dir) {
		t.Fatal("retired before its grace was over")
	}
	if store.Retire(ctx, 0) != 1 || exists(dir) || exists(usersOf(dir)) {
		t.Fatal("not retired once unused and rested")
	}
	if len(builder.forgotten) != 1 || builder.forgotten[0] != dir {
		t.Errorf("forgotten = %v; want %s", builder.forgotten, dir)
	}
}

func TestAWorkspaceThatIsGoneNoLongerKeepsItsTemplate(t *testing.T) {
	store := newStore(t, &fakeBuilder{})
	ctx := context.Background()
	if _, err := store.Take(ctx, web(), filepath.Join(t.TempDir(), "deleted-by-hand")); err != nil {
		t.Fatal(err)
	}
	if store.Retire(ctx, 0) != 1 {
		t.Error("a template whose only workspace is gone was kept")
	}
}

func TestEveryTemplateIsReadBackFromDisk(t *testing.T) {
	store := newStore(t, &fakeBuilder{})
	ctx := context.Background()
	withSubmodules := web()
	withSubmodules.Submodules = true
	github := Key{Repository: repo.Repo{Host: "github.com", Path: "oca/web"}, Commit: commit}
	for _, key := range []Key{web(), withSubmodules, github} {
		if _, err := store.Take(ctx, key, "/agents/a"); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(store.Dir, "github.com", "oca", "web", commit+".gitd-tmp-abc"), 0o755); err != nil {
		t.Fatal(err)
	}
	found := map[Key]bool{}
	for _, key := range store.All() {
		found[key] = true
	}
	if len(found) != 3 || !found[web()] || !found[withSubmodules] || !found[github] {
		t.Errorf("All = %v", store.All())
	}
}

func TestWhatACrashLeftAmongTheTemplatesIsSweptAndTheTemplatesKept(t *testing.T) {
	store := newStore(t, &fakeBuilder{})
	dir, err := store.Take(context.Background(), web(), "/agents/a")
	if err != nil {
		t.Fatal(err)
	}
	halfBuilt := dir + ".gitd-tmp-abc"
	halfWritten := usersOf(dir) + ".gitd-tmp-def"
	inside := filepath.Join(dir, "kept.gitd-tmp-ghi") // a template's own file, never looked at
	for _, path := range []string{halfBuilt} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{halfWritten, inside} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if removed := store.Sweep(); removed != 2 {
		t.Errorf("swept %d; want 2", removed)
	}
	if exists(halfBuilt) || exists(halfWritten) || !exists(dir) || !exists(usersOf(dir)) || !exists(inside) {
		t.Error("the sweep removed the wrong things")
	}
}
