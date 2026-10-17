package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// entries is a recorder that keeps what it is given.
type entries struct{ kept []*gitdv1.AuditEntry }

func (recorded *entries) Record(entry *gitdv1.AuditEntry) error {
	recorded.kept = append(recorded.kept, entry)
	return nil
}

func TestEveryPublishIsRecordedAndABranchOutsideThePrefixRefused(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	recorded := &entries{}
	settings, err := publishing.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Set(&gitdv1.PublishSettings{BranchPrefix: "agents/"}); err != nil {
		t.Fatal(err)
	}
	workspaces.UsePublishing(Publishing{Settings: settings, Recorder: recorded})
	ctx := context.Background()
	path := made(t, workspaces, roots, "agent-1", repository)

	attempts := []struct {
		path, branch string
		reason       git.Reason
	}{
		{path, "main", git.BranchRefused},
		{path, "agents/fix", git.NothingNew},
		{filepath.Join(roots, "missing"), "agents/fix", git.PathRefused},
	}
	for _, attempt := range attempts {
		_, err := workspaces.Publish(ctx, &gitdv1.PublishRequest{Path: attempt.path, Branch: attempt.branch})
		if failure, ok := git.AsFailure(err); !ok || failure.Reason != attempt.reason {
			t.Errorf("publish %s to %q: %v; want %v", filepath.Base(attempt.path), attempt.branch, err, attempt.reason)
		}
	}
	if len(recorded.kept) != len(attempts) {
		t.Fatalf("%d entries recorded for %d publishes", len(recorded.kept), len(attempts))
	}
	for index, entry := range recorded.kept {
		attempt := attempts[index]
		if entry.GetWorkspace() != attempt.path || entry.GetBranch() != attempt.branch ||
			entry.GetRefusal().GetReason() != attempt.reason || entry.GetRepository() != nil {
			t.Errorf("entry %d = %v; want a refusal of %q for %s", index, entry, attempt.branch, attempt.reason)
		}
	}
	if url := recorded.kept[0].GetUrl(); url != repository.HTTPS() {
		t.Errorf("the entry names %q; want the workspace's repository", url)
	}
}

func TestADonePublishIsRecordedWithWhatItPushed(t *testing.T) {
	recorded := &entries{}
	workspaces := &Workspaces{publishing: Publishing{Recorder: recorded}}
	pushed := &gitdv1.PublishedRepository{Url: "https://github.com/acme/web", Commit: "abc", Subjects: []string{"Fix"}}
	submodule := &gitdv1.PublishedRepository{Url: "https://github.com/acme/lib", Path: "lib", Commit: "def"}
	answer := &gitdv1.PublishResponse{Branch: "agents/fix", Repository: pushed, Submodules: []*gitdv1.PublishedRepository{submodule}}
	workspaces.record(&gitdv1.AuditEntry{Workspace: "/w", Branch: "agents/fix"}, answer, nil)
	entry := recorded.kept[0]
	if entry.GetRefusal() != nil || entry.GetRepository() != pushed || len(entry.GetSubmodules()) != 1 {
		t.Fatalf("entry = %v; want what was pushed and no refusal", entry)
	}
}

func TestAWorkspaceThatRenamesItsRepositoryIsNotOneGitdMade(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	other := repo.Repo{Host: "github.com", Path: "acme/other"}
	workspaces, roots := newTestWorkspaces(t, repository, other)
	ctx := context.Background()
	path := made(t, workspaces, roots, "agent-1", repository)
	if held, _ := workspaces.madeFor(ctx, path); held == nil || *held != repository {
		t.Fatalf("madeFor = %v; want %s", held, repository.HTTPS())
	}
	// The other repository's pool lends to the workspace too, as a submodule's would.
	if err := workspaces.pools.Use(ctx, other, path); err != nil {
		t.Fatal(err)
	}
	run(t, path, "config", "gitd.url", other.HTTPS())
	if held, _ := workspaces.madeFor(ctx, path); held != nil {
		t.Fatalf("a workspace naming another repository it borrows from is taken as one of %s", held.HTTPS())
	}
}

// newTestWorkspaces are workspaces under a root, with a pool for each repository, all of
// one upstream with one commit on main, and templates made by treeMounter.
func newTestWorkspaces(t *testing.T, repositories ...repo.Repo) (*Workspaces, string) {
	t.Helper()
	root := t.TempDir()
	upstream, pools, roots := filepath.Join(root, "upstream"), filepath.Join(root, "pools"), filepath.Join(root, "workspaces")
	run(t, root, "init", "-q", "-b", "main", upstream)
	if err := os.WriteFile(filepath.Join(upstream, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, upstream, "add", "README.md")
	run(t, upstream, "commit", "-q", "-m", "first")
	for _, repository := range repositories {
		run(t, root, "clone", "-q", "--bare", upstream, repository.Dir(pools))
	}
	if err := os.MkdirAll(roots, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := git.NewRunner("git")
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := credentials.NewKeys(filepath.Join(root, "state"), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	workspaces := New([]string{roots}, runner, pool.New(pools, runner, keys, locks, nil, 0), locks, lock.NewQueue(4, 4, time.Minute), time.Hour)
	workspaces.UseTemplates(template.New(filepath.Join(root, "templates"), locks, workspaces),
		&strategy.OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: treeMounter{}})
	return workspaces, roots
}

// made is a workspace of repository at roots/name, on main.
func made(t *testing.T, workspaces *Workspaces, roots, name string, repository repo.Repo) string {
	t.Helper()
	path := filepath.Join(roots, name)
	if _, err := workspaces.createFromTemplate(context.Background(), job{path: path, repository: repository, revision: onBranch("main")}); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOwnWorkspacesAreRecordedOnceForPoolsMadeBeforeGitdKeptThem(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	library := repo.Repo{Host: "github.com", Path: "acme/lib"}
	workspaces, roots := newTestWorkspaces(t, repository, library)
	ctx := context.Background()
	path := made(t, workspaces, roots, "agent-1", repository)
	if err := workspaces.pools.Use(ctx, library, path); err != nil {
		t.Fatal(err)
	}
	for _, dir := range workspaces.pools.All() {
		if err := os.Remove(filepath.Join(dir, "gitd-owned")); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	workspaces.RecordOwners(ctx)
	if !workspaces.pools.Owns(repository, path) {
		t.Error("the workspace is not recorded as its repository's own")
	}
	if workspaces.pools.Owns(library, path) {
		t.Error("the workspace is recorded as its submodule's own")
	}
}
