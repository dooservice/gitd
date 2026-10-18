package endpoint

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// run runs the git a sandbox has: the user's own, with nothing of gitd's environment.
func run(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=agent", "GIT_AUTHOR_EMAIL=agent@example.com",
		"GIT_COMMITTER_NAME=agent", "GIT_COMMITTER_EMAIL=agent@example.com")
	out, err := command.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func must(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := run(t, dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

// served is an endpoint over one pool of acme/web, with a README on main.
type served struct {
	repository repo.Repo
	tokens     *Tokens
	sandboxes  *Sandboxes
	server     *httptest.Server
	root       string
}

func serve(t *testing.T) served {
	t.Helper()
	root := t.TempDir()
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	upstream := filepath.Join(root, "upstream")
	must(t, root, "init", "-q", "-b", "main", upstream)
	if err := os.WriteFile(filepath.Join(upstream, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	must(t, upstream, "add", "README.md")
	must(t, upstream, "commit", "-q", "-m", "first")
	poolsDir := filepath.Join(root, "pools")
	must(t, root, "clone", "-q", "--bare", upstream, repository.Dir(poolsDir))

	runner := git.NewRunner("git")
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := credentials.NewKeys(filepath.Join(root, "state"), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pools := pool.New(poolsDir, runner, keys, locks, nil, 0)
	tokens := NewTokens()
	sandboxes := NewSandboxes(filepath.Join(root, "sandboxes"), runner, pools)
	server := httptest.NewServer(NewHandler(runner, tokens, sandboxes, 0))
	t.Cleanup(server.Close)
	return served{repository: repository, tokens: tokens, sandboxes: sandboxes, server: server, root: root}
}

// address is a clone URL carrying secret, as a sandbox is given it.
func (endpoint served) address(secret, repositoryPath string) string {
	parsed, _ := url.Parse(endpoint.server.URL)
	parsed.User = url.UserPassword("x", secret)
	parsed.Path = Prefix + "github.com/" + repositoryPath + ".git"
	return parsed.String()
}

func TestASandboxClonesWithItsTokenAndPushesToItsSandboxRepositoryOnly(t *testing.T) {
	ctx := context.Background()
	endpoint := serve(t)
	sandbox, err := endpoint.sandboxes.Make(ctx, endpoint.repository, "")
	if err != nil {
		t.Fatal(err)
	}
	secret, token, err := endpoint.tokens.Issue(endpoint.repository, sandbox.ID, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "gitd_") || token.Expires.Sub(token.Issued) != DefaultLife {
		t.Errorf("token = %+v, secret %q", token, secret)
	}

	work := filepath.Join(endpoint.root, "sandbox")
	must(t, endpoint.root, "clone", "-q", "--filter=blob:none", endpoint.address(secret, "acme/web"), work)
	if data, err := os.ReadFile(filepath.Join(work, "README.md")); err != nil || string(data) != "hi\n" {
		t.Fatalf("the clone has %q, %v", data, err)
	}
	if err := os.WriteFile(filepath.Join(work, "fix.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	must(t, work, "add", "fix.txt")
	must(t, work, "commit", "-q", "-m", "fix")
	must(t, work, "push", "-q", "origin", "HEAD:refs/heads/agents/fix")
	pushed := must(t, endpoint.sandboxes.Path(sandbox.ID), "rev-parse", "refs/heads/agents/fix")
	if pushed != must(t, work, "rev-parse", "HEAD") {
		t.Errorf("the sandbox repository holds %s", pushed)
	}
	if _, err := run(t, filepath.Join(endpoint.root, "pools", "github.com", "acme", "web.git"), "rev-parse", "--verify", "refs/heads/agents/fix"); err == nil {
		t.Error("a push reached the pool")
	}
	if out, err := run(t, work, "push", "origin", "--delete", "main"); err == nil {
		t.Errorf("a sandbox deleted a branch: %s", out)
	}
}

func TestATokenOpensOnlyItsRepositoryWhileAliveAndAsItWasIssued(t *testing.T) {
	ctx := context.Background()
	endpoint := serve(t)
	sandbox, err := endpoint.sandboxes.Make(ctx, endpoint.repository, "")
	if err != nil {
		t.Fatal(err)
	}
	reading, _, err := endpoint.tokens.Issue(endpoint.repository, sandbox.ID, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	cloned := func(secret, path, into string) error {
		_, err := run(t, endpoint.root, "clone", "-q", endpoint.address(secret, path), filepath.Join(endpoint.root, into))
		return err
	}
	if cloned("", "acme/web", "none") == nil {
		t.Error("cloned without a token")
	}
	if cloned("gitd_not-a-token", "acme/web", "made-up") == nil {
		t.Error("cloned with a token gitd never issued")
	}
	if cloned(reading, "acme/other", "other") == nil {
		t.Error("a token opened another repository")
	}
	if err := cloned(reading, "acme/web", "read"); err != nil {
		t.Fatalf("a reading token did not clone: %v", err)
	}
	work := filepath.Join(endpoint.root, "read")
	must(t, work, "commit", "-q", "--allow-empty", "-m", "try")
	if _, err := run(t, work, "push", "origin", "HEAD:refs/heads/agents/try"); err == nil {
		t.Error("a reading token pushed")
	}

	writing, token, err := endpoint.tokens.Issue(endpoint.repository, sandbox.ID, true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := endpoint.tokens.Revoke(token.ID); !ok {
		t.Fatal("the token was not revoked")
	}
	if cloned(writing, "acme/web", "revoked") == nil {
		t.Error("a revoked token cloned")
	}
	expired, _, err := endpoint.tokens.Issue(endpoint.repository, sandbox.ID, true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.tokens.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if cloned(expired, "acme/web", "expired") == nil {
		t.Error("an expired token cloned")
	}
	if alive := endpoint.tokens.Alive(); len(alive) != 1 || alive[0].Writable {
		t.Errorf("tokens alive = %v; want the reading one only, issued for an hour", alive)
	}
	if _, _, err := endpoint.tokens.Issue(endpoint.repository, sandbox.ID, true, 48*time.Hour); err == nil {
		t.Error("a token was issued for two days")
	}
}

func TestASandboxRepositoryIsRetiredOnlyWhenNoTokenHoldsItAndNothingPushed(t *testing.T) {
	ctx := context.Background()
	endpoint := serve(t)
	held, err := endpoint.sandboxes.Make(ctx, endpoint.repository, "")
	if err != nil {
		t.Fatal(err)
	}
	left, err := endpoint.sandboxes.Make(ctx, endpoint.repository, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := endpoint.tokens.Issue(endpoint.repository, held.ID, true, 0); err != nil {
		t.Fatal(err)
	}
	if retired := endpoint.sandboxes.Retire(ctx, time.Hour, endpoint.tokens); retired != 0 {
		t.Errorf("retired %d sandboxes used just now", retired)
	}
	if retired := endpoint.sandboxes.Retire(ctx, 0, endpoint.tokens); retired != 1 {
		t.Errorf("retired %d; want the one no token holds", retired)
	}
	if _, err := endpoint.sandboxes.Get(ctx, left.ID); err == nil {
		t.Error("the retired sandbox is still there")
	}
	if _, err := endpoint.sandboxes.Get(ctx, held.ID); err != nil {
		t.Errorf("the held sandbox is gone: %v", err)
	}
	if _, err := endpoint.sandboxes.Get(ctx, "../etc"); err == nil {
		t.Error("a sandbox was looked for outside the sandboxes")
	}
}
