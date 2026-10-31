package pool

import (
	"context"
	"encoding/pem"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/repo"
)

// host is a repository served over HTTPS by git's own smart HTTP server, taking pushes, with
// a pool of it that trusts the server's certificate and a token that opens it.
type host struct {
	pools      *Pools
	repository repo.Repo
	served     string
	work       string
	token      []credentials.Credential
}

func newHost(t *testing.T) host {
	t.Helper()
	root := t.TempDir()
	served := filepath.Join(root, "served", "acme", "web")
	gitIn(t, root, "init", "-q", "--bare", "-b", "main", served)
	work := filepath.Join(root, "work")
	gitIn(t, root, "clone", "-q", served, work)
	write(t, filepath.Join(work, "README.md"), "hi\n")
	gitIn(t, work, "add", "-A")
	gitIn(t, work, "commit", "-q", "-m", "first")
	gitIn(t, work, "push", "-q", "origin", "main")

	runner := git.NewRunner("git")
	server := httptest.NewTLSServer(runner.HTTPBackend(filepath.Join(root, "served"), []string{"http.receivepack=true"}))
	t.Cleanup(server.Close)
	address, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(address.Port())
	certificate := filepath.Join(root, "server.pem")
	if err := os.WriteFile(certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o644); err != nil {
		t.Fatal(err)
	}

	repository := repo.Repo{Host: "127.0.0.1", Path: "acme/web"}
	pools := filepath.Join(root, "pools")
	gitIn(t, root, "clone", "-q", "--bare", served, repository.Dir(pools))
	gitIn(t, repository.Dir(pools), "config", "http.sslCAInfo", certificate)
	locks, _ := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	keys, err := credentials.NewKeys(filepath.Join(root, "state"), nil, repo.Hosts{"127.0.0.1": {HTTPSPort: port}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return host{
		pools: New(pools, runner, keys, locks, nil, 0), repository: repository, served: served, work: work,
		token: []credentials.Credential{{Prefix: "https://127.0.0.1/acme", Token: "t"}},
	}
}

// commit commits a file in the work tree and stages it for branch, saying what it brings.
func (host host) commit(t *testing.T, name, branch string) []string {
	t.Helper()
	write(t, filepath.Join(host.work, name), name+"\n")
	gitIn(t, host.work, "add", "-A")
	gitIn(t, host.work, "commit", "-q", "-m", "add "+name)
	return host.stage(t, branch)
}

func (host host) stage(t *testing.T, branch string) []string {
	t.Helper()
	_, subjects, err := host.pools.Stage(context.Background(), host.repository, filepath.Join(host.work, ".git"), branch)
	if err != nil {
		t.Fatal(err)
	}
	return subjects
}

// headOf is the commit a branch of the served repository is on.
func (host host) headOf(t *testing.T, branch string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", host.served, "rev-parse", "refs/heads/"+branch).Output()
	if err != nil {
		t.Fatalf("the host has no branch %s: %v", branch, err)
	}
	return strings.TrimSpace(string(out))
}

func TestABranchIsPublishedNewAndMovedOnlyForwardWhenAskedTo(t *testing.T) {
	host := newHost(t)
	ctx := context.Background()
	branch := "agents/fix"
	host.commit(t, "a.go", branch)
	if err := host.pools.Push(ctx, host.repository, branch, host.token, false); err != nil {
		t.Fatal(err)
	}

	if subjects := host.commit(t, "b.go", branch); !slices.Equal(subjects, []string{"add b.go"}) {
		t.Fatalf("staged %v; want only what the branch does not have", subjects)
	}
	err := host.pools.Push(ctx, host.repository, branch, host.token, false)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.BranchExists || !strings.Contains(failure.GitSaid, "update") {
		t.Fatalf("a push to a branch that exists, without update = %v", err)
	}
	if err := host.pools.Push(ctx, host.repository, branch, host.token, true); err != nil {
		t.Fatal(err)
	}
	if head := host.headOf(t, branch); head != strings.TrimSpace(run(t, host.work, "rev-parse", "HEAD")) {
		t.Errorf("the branch is on %s; want the work's commit", head)
	}
	summary, err := host.pools.Summarize(ctx, host.repository, branch)
	if err != nil || !slices.Equal(summary.Subjects, []string{"add b.go", "add a.go"}) || summary.Files != 2 || summary.Added != 2 {
		t.Errorf("summary = %+v, %v; want both publishes", summary, err)
	}

	// Someone pushes to the branch on the host; the work, without that, is not ahead of it.
	other := filepath.Join(t.TempDir(), "other")
	gitIn(t, filepath.Dir(other), "clone", "-q", "-b", branch, host.served, other)
	write(t, filepath.Join(other, "theirs.go"), "theirs\n")
	gitIn(t, other, "add", "-A")
	gitIn(t, other, "commit", "-q", "-m", "theirs")
	gitIn(t, other, "push", "-q", "origin", branch)
	theirs := host.headOf(t, branch)
	host.commit(t, "c.go", branch)
	err = host.pools.Push(ctx, host.repository, branch, host.token, true)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.BranchExists || !strings.Contains(failure.GitSaid, "only forward") {
		t.Fatalf("a push that is not a fast-forward = %v", err)
	}
	if head := host.headOf(t, branch); head != theirs {
		t.Error("a push that was not a fast-forward moved the branch")
	}
}

func TestABranchHoldsWhatWasPublishedToItAndNothingElse(t *testing.T) {
	host := newHost(t)
	ctx := context.Background()
	branch := "agents/fix"
	host.commit(t, "a.go", branch)
	if holds, err := host.pools.Holds(ctx, host.repository, branch); err != nil || holds {
		t.Fatalf("Holds before any publish = %v, %v", holds, err)
	}
	if err := host.pools.Push(ctx, host.repository, branch, host.token, false); err != nil {
		t.Fatal(err)
	}
	if subjects := host.stage(t, branch); len(subjects) != 0 {
		t.Fatalf("staged %v again", subjects)
	}
	if holds, err := host.pools.Holds(ctx, host.repository, branch); err != nil || !holds {
		t.Errorf("Holds of what was published = %v, %v", holds, err)
	}
	gitIn(t, host.work, "reset", "-q", "--hard", "HEAD~1")
	if subjects := host.stage(t, branch); len(subjects) != 0 {
		t.Fatalf("staged %v from main", subjects)
	}
	if holds, err := host.pools.Holds(ctx, host.repository, branch); err != nil || holds {
		t.Errorf("Holds of nothing but main = %v, %v", holds, err)
	}
}

// run runs git in dir and returns what it wrote.
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}
