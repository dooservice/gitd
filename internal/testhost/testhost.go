// Package testhost serves a repository over HTTPS with git's own smart HTTP server, taking
// pushes, for tests that publish: a git host on this machine, with a certificate a pool can
// be told to trust.
package testhost

import (
	"encoding/pem"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Host is acme/web served at 127.0.0.1, with main holding one commit, and a clone of it,
// Work, to commit on the host's side.
type Host struct {
	Repository  repo.Repo
	Served      string
	Work        string
	Certificate string
	Hosts       repo.Hosts
	Token       []credentials.Credential
}

// New serves a repository for the length of the test.
func New(t *testing.T) Host {
	t.Helper()
	root := t.TempDir()
	served := filepath.Join(root, "served", "acme", "web")
	Git(t, root, "init", "-q", "--bare", "-b", "main", served)
	work := filepath.Join(root, "work")
	Git(t, root, "clone", "-q", served, work)
	Write(t, filepath.Join(work, "README.md"), "hi\n")
	Git(t, work, "add", "-A")
	Git(t, work, "commit", "-q", "-m", "first")
	Git(t, work, "push", "-q", "origin", "main")

	server := httptest.NewTLSServer(git.NewRunner("git").HTTPBackend(filepath.Join(root, "served"), []string{"http.receivepack=true"}))
	t.Cleanup(server.Close)
	address, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(address.Port())
	certificate := filepath.Join(root, "server.pem")
	Write(t, certificate, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})))
	return Host{
		Repository:  repo.Repo{Host: "127.0.0.1", Path: "acme/web"},
		Served:      served,
		Work:        work,
		Certificate: certificate,
		Hosts:       repo.Hosts{"127.0.0.1": {HTTPSPort: port}},
		Token:       []credentials.Credential{{Prefix: "https://127.0.0.1/acme", Token: "t"}},
	}
}

// Pool clones the served repository, bare, into the pool of it under pools, trusting the
// server's certificate.
func (host Host) Pool(t *testing.T, pools string) string {
	t.Helper()
	dir := host.Repository.Dir(pools)
	Git(t, filepath.Dir(pools), "clone", "-q", "--bare", host.Served, dir)
	Git(t, dir, "config", "http.sslCAInfo", host.Certificate)
	return dir
}

// ShallowPool clones the served repository's main tip only into the pool of it under
// pools, as gitd makes a pool unless the whole history is wanted.
func (host Host) ShallowPool(t *testing.T, pools string) string {
	t.Helper()
	dir := host.Repository.Dir(pools)
	Git(t, filepath.Dir(pools), "clone", "-q", "--bare", "--depth", "1", "file://"+host.Served, dir)
	Git(t, dir, "config", "http.sslCAInfo", host.Certificate)
	return dir
}

// Commit writes a file in Work, commits it and pushes it to the host's main.
func (host Host) Commit(t *testing.T, name, content string) {
	t.Helper()
	Write(t, filepath.Join(host.Work, name), content)
	Git(t, host.Work, "add", "-A")
	Git(t, host.Work, "commit", "-q", "-m", "change "+name)
	Git(t, host.Work, "push", "-q", "origin", "main")
}

// CommitAt is Commit, made at when, as git reads a date: a history with an age.
func (host Host) CommitAt(t *testing.T, name, content, when string) {
	t.Helper()
	Write(t, filepath.Join(host.Work, name), content)
	Git(t, host.Work, "add", "-A")
	command := exec.Command("git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "change "+name)
	command.Dir = host.Work
	command.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("committing at %s: %v\n%s", when, err, out)
	}
	Git(t, host.Work, "push", "-q", "origin", "main")
}

// Head is the commit a branch of the served repository is on.
func (host Host) Head(t *testing.T, branch string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", host.Served, "rev-parse", "refs/heads/"+branch).Output()
	if err != nil {
		t.Fatalf("the host has no branch %s: %v", branch, err)
	}
	return strings.TrimSpace(string(out))
}

// Git runs git in dir as a person would, with an identity, and fails the test when it fails.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Write writes content to path, making its directory.
func Write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
