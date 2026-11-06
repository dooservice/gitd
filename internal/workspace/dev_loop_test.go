package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/testhost"
)

// devLoop is gitd as a developer's machine runs it, against a git host on this machine.
type devLoop struct {
	host       testhost.Host
	workspaces *Workspaces
	path       string
	token      []*gitdv1.Credential
}

func newDevLoop(t *testing.T) devLoop {
	t.Helper()
	host := testhost.New(t)
	root := t.TempDir()
	pools, roots := filepath.Join(root, "pools"), filepath.Join(root, "workspaces")
	host.Pool(t, pools)
	if err := os.MkdirAll(roots, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := git.NewRunner("git")
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := credentials.NewKeys(filepath.Join(root, "state"), nil, host.Hosts, nil)
	if err != nil {
		t.Fatal(err)
	}
	fetches := lock.NewQueue(4, 4, time.Minute)
	workspaces := New([]string{roots}, runner, pool.New(pools, runner, keys, locks, fetches, 0), locks, fetches, time.Hour)
	token := []*gitdv1.Credential{{UrlPrefix: "https://127.0.0.1/acme", Secret: &gitdv1.Credential_HttpsToken{HttpsToken: "t"}}}
	return devLoop{host: host, workspaces: workspaces, path: filepath.Join(roots, "agent-1"), token: token}
}

func (loop devLoop) commit(t *testing.T, name, content string) {
	t.Helper()
	testhost.Write(t, filepath.Join(loop.path, name), content)
	testhost.Git(t, loop.path, "add", "-A")
	testhost.Git(t, loop.path, "commit", "-q", "-m", "work on "+name)
}

func (loop devLoop) publish(t *testing.T) *gitdv1.PublishResponse {
	t.Helper()
	answer, err := loop.workspaces.Publish(context.Background(), &gitdv1.PublishRequest{Path: loop.path, Credentials: loop.token})
	if err != nil {
		t.Fatal(err)
	}
	return answer
}

func TestAWorkspaceOnItsOwnBranchPublishesItTakesMainInAndPublishesAgain(t *testing.T) {
	loop := newDevLoop(t)
	ctx := context.Background()
	result, err := loop.workspaces.Prepare(ctx, &gitdv1.PrepareRequest{
		Url: loop.host.Repository.HTTPS(), Path: loop.path, NewBranch: "agents/work", Credentials: loop.token,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Branch != "agents/work" || testhost.Git(t, loop.path, "symbolic-ref", "--short", "HEAD") != "agents/work" {
		t.Fatalf("made on %q; want its own branch agents/work", result.Branch)
	}

	loop.commit(t, "a.go", "a\n")
	if answer := loop.publish(t); answer.GetBranch() != "agents/work" {
		t.Fatalf("published %q; want the branch the workspace is on", answer.GetBranch())
	}

	loop.host.Commit(t, "main.go", "main\n")
	loop.commit(t, "b.go", "b\n")
	merged, err := loop.workspaces.Update(ctx, &gitdv1.UpdateRequest{Path: loop.path, Merge: "main", Credentials: loop.token}, nil)
	if err != nil {
		t.Fatal(err)
	}
	parents := strings.Fields(testhost.Git(t, loop.path, "rev-list", "--parents", "-n", "1", "HEAD"))
	if merged.Commit != parents[0] || len(parents) != 3 || merged.Branch != "agents/work" {
		t.Fatalf("after the merge HEAD is %v, on %q; want a merge commit on agents/work", parents, merged.Branch)
	}
	if _, err := os.Stat(filepath.Join(loop.path, "main.go")); err != nil {
		t.Fatal("main's new file is not in the workspace")
	}

	loop.publish(t)
	if head := loop.host.Head(t, "agents/work"); head != merged.Commit {
		t.Errorf("the published branch is on %s; want the merge %s, moved forward without asking", head, merged.Commit)
	}

	listed, err := loop.workspaces.List(ctx)
	if err != nil || len(listed.GetWorkspaces()) != 1 {
		t.Fatalf("listed %v, %v; want the one workspace", listed, err)
	}
	only := listed.GetWorkspaces()[0]
	if only.GetPath() != loop.path || !slices.Equal(only.GetPublished(), []string{"agents/work"}) ||
		only.GetDescription().GetBranch() != "agents/work" {
		t.Errorf("listed %v; want it on agents/work, with agents/work published", only)
	}
}

func TestAMergeThatConflictsLeavesTheConflictMarkedAndSaysSo(t *testing.T) {
	loop := newDevLoop(t)
	ctx := context.Background()
	if _, err := loop.workspaces.Prepare(ctx, &gitdv1.PrepareRequest{
		Url: loop.host.Repository.HTTPS(), Path: loop.path, NewBranch: "agents/work", Credentials: loop.token,
	}, nil); err != nil {
		t.Fatal(err)
	}
	loop.commit(t, "README.md", "the agent's\n")
	loop.host.Commit(t, "README.md", "main's\n")
	_, err := loop.workspaces.Update(ctx, &gitdv1.UpdateRequest{Path: loop.path, Merge: "main", Credentials: loop.token}, nil)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.Conflict || !strings.Contains(failure.GitSaid, "README.md") {
		t.Fatalf("a conflicting merge = %v; want REASON_CONFLICT naming README.md", err)
	}
	if data, _ := os.ReadFile(filepath.Join(loop.path, "README.md")); !strings.Contains(string(data), "<<<<<<<") {
		t.Errorf("README.md holds %q; want the conflict marked", data)
	}
}

func TestAPublishMovesForwardWithoutAskingOnlyABranchThisWorkspacePublished(t *testing.T) {
	loop := newDevLoop(t)
	ctx := context.Background()
	other := filepath.Join(filepath.Dir(loop.path), "agent-2")
	for path, own := range map[string]string{loop.path: "agents/one", other: "agents/two"} {
		if _, err := loop.workspaces.Prepare(ctx, &gitdv1.PrepareRequest{
			Url: loop.host.Repository.HTTPS(), Path: path, NewBranch: own, Credentials: loop.token,
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	loop.commit(t, "a.go", "a\n")
	loop.publish(t)
	second := devLoop{host: loop.host, workspaces: loop.workspaces, path: other, token: loop.token}
	second.commit(t, "b.go", "b\n")
	_, err := loop.workspaces.Publish(ctx, &gitdv1.PublishRequest{Path: other, Branch: "agents/one", Credentials: loop.token})
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.BranchExists {
		t.Fatalf("another workspace publishing to agents/one = %v; want BRANCH_EXISTS: only its own publishes move without asking", err)
	}
}

func TestMergingABranchTheRepositoryHasNotIsNotFound(t *testing.T) {
	loop := newDevLoop(t)
	ctx := context.Background()
	if _, err := loop.workspaces.Prepare(ctx, &gitdv1.PrepareRequest{
		Url: loop.host.Repository.HTTPS(), Path: loop.path, NewBranch: "agents/work", Credentials: loop.token,
	}, nil); err != nil {
		t.Fatal(err)
	}
	_, err := loop.workspaces.Update(ctx, &gitdv1.UpdateRequest{Path: loop.path, Merge: "nope", Credentials: loop.token}, nil)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.NotFound {
		t.Fatalf("merging a branch that is not there = %v; want NOT_FOUND", err)
	}
}
