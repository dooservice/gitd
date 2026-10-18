package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/dooservice/gitd/internal/apikeys"
	"github.com/dooservice/gitd/internal/client"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/endpoint"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// tenantPart is one tenant behind a gateway: an endpoint over its own pool of one
// repository, a sandbox repository of it, a token, and an API that answers who it is.
type tenantPart struct {
	tenant *Tenant
	secret string
}

func newTenantPart(t *testing.T, name, path string) tenantPart {
	t.Helper()
	root := t.TempDir()
	repository := repo.Repo{Host: "github.com", Path: path}
	upstream := filepath.Join(root, "upstream")
	gitIn(t, root, "init", "-q", "-b", "main", upstream)
	gitIn(t, upstream, "commit", "-q", "--allow-empty", "-m", name)
	poolsDir := filepath.Join(root, "pools")
	gitIn(t, root, "clone", "-q", "--bare", upstream, repository.Dir(poolsDir))
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
	tokens := endpoint.NewTokens()
	sandboxes := endpoint.NewSandboxes(filepath.Join(root, "sandboxes"), runner, pools)
	sandbox, err := sandboxes.Make(context.Background(), repository, "")
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := tokens.Issue(repository, sandbox.ID, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(root, "key")
	if err := os.WriteFile(keyFile, []byte(name+"-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	served := &Server{Version: name, Runner: runner, Operation: time.Minute, Metrics: metrics.New(func() float64 { return 0 }), Totals: func() (int, int64) { return 0, 0 }}
	return tenantPart{
		tenant: &Tenant{Name: name, Key: apikeys.NewFile(keyFile), API: served.Routes(), Git: endpoint.NewHandler(runner, tokens, sandboxes, 0)},
		secret: secret,
	}
}

func gitIn(t *testing.T, dir string, args ...string) error {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := command.CombinedOutput()
	if err != nil && args[0] != "ls-remote" {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return err
}

// TestOnTheNetworkATokenOrAKeyOpensItsOwnTenantOnly serves two tenants on one network
// address and tries each one's token and key on the other's.
func TestOnTheNetworkATokenOrAKeyOpensItsOwnTenantOnly(t *testing.T) {
	acme, globex := newTenantPart(t, "acme", "acme/web"), newTenantPart(t, "globex", "globex/app")
	gateway := &Gateway{Tenants: []*Tenant{acme.tenant, globex.tenant}}
	served := httptest.NewServer(http.HandlerFunc(gateway.network))
	t.Cleanup(served.Close)
	reads := func(secret, path string) bool {
		address, _ := url.Parse(served.URL)
		address.User = url.UserPassword("x", secret)
		address.Path = endpoint.Prefix + "github.com/" + path + ".git"
		return gitIn(t, t.TempDir(), "ls-remote", address.String()) == nil
	}
	if !reads(acme.secret, "acme/web") || !reads(globex.secret, "globex/app") {
		t.Fatal("a tenant's token did not read its own repository")
	}
	if reads(acme.secret, "globex/app") || reads(globex.secret, "acme/web") || reads("", "acme/web") {
		t.Error("a token read another tenant's repository, or none was needed")
	}

	ctx := context.Background()
	for name, want := range map[string]string{"acme-key": "acme", "globex-key": "globex"} {
		called, err := client.Open(client.Options{URL: served.URL, Key: name})
		if err != nil {
			t.Fatal(err)
		}
		info, err := called.Info(ctx)
		if err != nil || info.GetVersion() != want {
			t.Errorf("%s reached %q, %v; want %s", name, info.GetVersion(), err, want)
		}
	}
	stranger, err := client.Open(client.Options{URL: served.URL, Key: acme.secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stranger.Info(ctx); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("an endpoint token opened the API: %v", err)
	}
}
