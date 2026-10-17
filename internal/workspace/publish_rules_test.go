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
	root := t.TempDir()
	upstream, pools, roots := filepath.Join(root, "upstream"), filepath.Join(root, "pools"), filepath.Join(root, "workspaces")
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	run(t, root, "init", "-q", "-b", "main", upstream)
	if err := os.WriteFile(filepath.Join(upstream, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, upstream, "add", "README.md")
	run(t, upstream, "commit", "-q", "-m", "first")
	run(t, root, "clone", "-q", "--bare", upstream, repository.Dir(pools))
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
	recorded := &entries{}
	workspaces.UsePublishing(Publishing{BranchPrefix: "agents/", Recorder: recorded})
	ctx := context.Background()
	path := filepath.Join(roots, "agent-1")
	if _, err := workspaces.createFromTemplate(ctx, job{path: path, repository: repository, revision: onBranch("main")}); err != nil {
		t.Fatal(err)
	}

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
