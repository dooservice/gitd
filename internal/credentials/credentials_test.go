package credentials

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/repo"
)

func TestThePrefixOfAKeyAndOfATokenAreWrittenTheSameWay(t *testing.T) {
	for prefix, want := range map[string]string{
		"git@github.com:Acme/":                 "https://github.com/acme",
		"https://github.com/acme/":             "https://github.com/acme",
		"https://github.com/acme/x.git":        "https://github.com/acme/x",
		"ssh://git@Git.Example.com:2222/acme/": "https://git.example.com/acme",
		"https://Git.Example.com:3000/acme":    "https://git.example.com/acme",
		"forgejo@git.example.com:acme":         "https://git.example.com/acme",
	} {
		if got := CanonicalPrefix(prefix); got != want {
			t.Errorf("CanonicalPrefix(%q) = %q", prefix, got)
		}
	}
}

func TestARepositoryIsReadWithTheMostSpecificCredentialAndNoOther(t *testing.T) {
	keys := &Keys{knownHosts: "/state/known_hosts"}
	repository, _ := repo.Of("git@github.com:acme/web.git")
	given := []Credential{
		{Prefix: "", Key: "/keys/project"},
		{Prefix: "https://github.com/acme/web", Key: "/keys/web"},
		{Prefix: "https://github.com/other/web", Token: "elsewhere"},
	}
	access := accessOf(t, keys, repository, given)
	if access.URL != "git@github.com:acme/web.git" || !strings.Contains(access.Env[0], "'/keys/web'") {
		t.Errorf("own key = %+v", access)
	}
	public, _ := repo.Of("git@github.com:OCA/web.git")
	if access := accessOf(t, keys, public, given); !strings.Contains(access.Env[0], "'/keys/project'") {
		t.Errorf("project key = %+v", access)
	}
	private, _ := repo.Of("git@github.com:other/web.git")
	if access := accessOf(t, keys, private, given); access.URL != "https://github.com/other/web" || !access.Offered {
		t.Errorf("token = %+v", access)
	}
	if access := accessOf(t, keys, public, nil); access.Offered || access.Env != nil {
		t.Errorf("nothing = %+v", access)
	}
}

func TestATokenTravelsInAHeaderScopedToOneRepository(t *testing.T) {
	keys := &Keys{}
	repository, _ := repo.Of("https://github.com/acme/web")
	access := keys.access(repository, Credential{Token: "secret"})
	joined := strings.Join(access.Env, "\n")
	if !strings.Contains(joined, "http.https://github.com/acme/web.extraHeader") || strings.Contains(joined, "secret") {
		t.Errorf("env = %v", access.Env)
	}
	if strings.Contains(access.URL, "secret") {
		t.Errorf("the token is in the address: %q", access.URL)
	}
}

func TestATokenGoesWithTheUserItsHostExpects(t *testing.T) {
	keys := &Keys{}
	for address, user := range map[string]string{
		"https://github.com/acme/web":        "x-access-token",
		"https://gitlab.com/acme/tools/web":  "oauth2",
		"https://bitbucket.org/acme/web":     "x-token-auth",
		"https://gitea.example.com/acme/web": "x-access-token",
	} {
		repository, _ := repo.Of(address)
		access := keys.access(repository, Credential{Token: "secret"})
		want := base64.StdEncoding.EncodeToString([]byte(user + ":secret"))
		if !strings.Contains(strings.Join(access.Env, "\n"), want) {
			t.Errorf("%s: env = %v; want user %q", address, access.Env, user)
		}
	}
	repository, _ := repo.Of("https://bitbucket.org/acme/web")
	access := keys.access(repository, Credential{Token: "secret", User: "josue"})
	if !strings.Contains(strings.Join(access.Env, "\n"), base64.StdEncoding.EncodeToString([]byte("josue:secret"))) {
		t.Errorf("a given user was not used: %v", access.Env)
	}
}

func TestTheConfiguredHostKeysAreTrustedBesidesTheBuiltInOnes(t *testing.T) {
	dir := t.TempDir()
	extra := filepath.Join(dir, "mine")
	if err := os.WriteFile(extra, []byte("git.example.com ssh-ed25519 AAAAexample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keys, err := NewKeys(filepath.Join(dir, "state"), []string{extra}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(keys.knownHosts)
	for _, host := range []string{"github.com ", "gitlab.com ", "bitbucket.org ", "codeberg.org ", "git.example.com "} {
		if !strings.Contains(string(written), "\n"+host) && !strings.HasPrefix(string(written), host) {
			t.Errorf("no key for %s", host)
		}
	}
	if _, err := NewKeys(filepath.Join(dir, "state"), []string{filepath.Join(dir, "missing")}, nil, nil); err == nil {
		t.Error("a missing known hosts file was not refused")
	}
}

func TestARepositoryIsReadOnThePortsAndAsTheUserOfItsHost(t *testing.T) {
	keys, err := NewKeys(t.TempDir(), nil, repo.Hosts{
		"git.example.com":   {HTTPSPort: 3000, SSHPort: 2222},
		"forge.example.com": {SSHUser: "forgejo"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for address, want := range map[string]string{
		"https://git.example.com/acme/web":         "ssh://git@git.example.com:2222/acme/web.git",
		"ssh://git@git.example.com:2222/acme/web":  "ssh://git@git.example.com:2222/acme/web.git",
		"https://forge.example.com/acme/tools/web": "forgejo@forge.example.com:acme/tools/web.git",
		"https://github.com/acme/web":              "git@github.com:acme/web.git",
	} {
		repository, _ := repo.Of(address)
		access := accessOf(t, keys, repository, []Credential{{Key: "/keys/web"}})
		if access.URL != want {
			t.Errorf("%s: URL = %q; want %q", address, access.URL, want)
		}
	}
}

func TestATokenIsSentToTheAddressOnTheHostsPort(t *testing.T) {
	keys, err := NewKeys(t.TempDir(), nil, repo.Hosts{"git.example.com": {HTTPSPort: 3000}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository, _ := repo.Of("ssh://git@git.example.com:2222/acme/web.git")
	given := []Credential{{Prefix: CanonicalPrefix("https://git.example.com:3000/acme"), Token: "secret"}}
	access := accessOf(t, keys, repository, given)
	if access.URL != "https://git.example.com:3000/acme/web" || !access.Offered {
		t.Fatalf("access = %+v", access)
	}
	if !strings.Contains(strings.Join(access.Env, "\n"), "http.https://git.example.com:3000/acme/web.extraHeader") {
		t.Errorf("the header is not scoped to the address read: %v", access.Env)
	}
}

func TestACredentialGitdKeepsOpensWhatTheRequestGivesNoneFor(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "github")
	if err := os.WriteFile(tokenFile, []byte("kept-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITD_TEST_TOKEN", "env-secret")
	stored, err := Stored([]config.Credential{
		{Prefix: "https://github.com/acme", TokenFile: tokenFile},
		{Prefix: "git@gitlab.com:acme/", TokenEnv: "GITD_TEST_TOKEN", TokenUser: "oauth2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := NewKeys(filepath.Join(dir, "state"), nil, nil, stored)
	if err != nil {
		t.Fatal(err)
	}
	header := func(access Access) string { return strings.Join(access.Env, "\n") }

	github, _ := repo.Of("https://github.com/acme/web")
	if access := accessOf(t, keys, github, nil); !access.Offered || !strings.Contains(header(access), base64.StdEncoding.EncodeToString([]byte("x-access-token:kept-secret"))) {
		t.Errorf("the kept token was not used: %+v", access)
	}
	gitlab, _ := repo.Of("https://gitlab.com/acme/tools/web")
	if access := accessOf(t, keys, gitlab, nil); !strings.Contains(header(access), base64.StdEncoding.EncodeToString([]byte("oauth2:env-secret"))) {
		t.Errorf("the token from the environment was not used: %+v", access)
	}
	given := []Credential{{Prefix: "https://github.com/acme", Token: "request-secret"}}
	if access := accessOf(t, keys, github, given); !strings.Contains(header(access), base64.StdEncoding.EncodeToString([]byte("x-access-token:request-secret"))) {
		t.Errorf("the request's credential did not win over the kept one: %+v", access)
	}
	other, _ := repo.Of("https://github.com/other/web")
	if access := accessOf(t, keys, other, nil); access.Offered {
		t.Errorf("a kept credential opened a repository outside its prefix: %+v", access)
	}
}

func TestACredentialWhoseSecretCannotBeReadStopsTheStart(t *testing.T) {
	t.Setenv("GITD_EMPTY", "")
	for _, setting := range []config.Credential{
		{Prefix: "https://github.com/acme", TokenFile: "/nonexistent/token"},
		{Prefix: "https://github.com/acme", TokenEnv: "GITD_EMPTY"},
		{Prefix: "https://github.com/acme", SSHKey: "/nonexistent/key"},
	} {
		if _, err := Stored([]config.Credential{setting}); err == nil {
			t.Errorf("accepted %+v", setting)
		}
	}
}

func TestATokenIsNeverSentToAHostItsPrefixDoesNotName(t *testing.T) {
	keys := &Keys{knownHosts: "/state/known_hosts"}
	internal := repo.Repo{Host: "169.254.169.254", Path: "latest/meta-data"}
	for _, prefix := range []string{"", "https://", "git@:"} {
		given := []Credential{{Prefix: CanonicalPrefix(prefix), Token: "secret"}}
		access := accessOf(t, keys, internal, given)
		if access.Offered || strings.Contains(strings.Join(access.Env, " "), base64.StdEncoding.EncodeToString([]byte("x-access-token:secret"))) {
			t.Errorf("a token with prefix %q is offered to %s", prefix, internal.Host)
		}
	}
	if _, err := Stored([]config.Credential{{Prefix: "", TokenEnv: "HOME"}}); err == nil {
		t.Error("a kept token with no prefix is accepted")
	}
}

// changing is credentials that change while gitd runs, as connections made through the API do.
type changing struct{ credentials []Credential }

func (source *changing) Credentials() []Credential { return source.credentials }

func TestACredentialKeptWhileGitdRunsIsUsedAtOnce(t *testing.T) {
	keys, err := NewKeys(t.TempDir(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := &changing{}
	keys.AlsoKeep(source)
	repository, _ := repo.Of("https://gitlab.example.com/acme/tools")
	if accessOf(t, keys, repository, nil).Offered {
		t.Fatal("a credential was offered before any was kept")
	}
	source.credentials = []Credential{KeptToken("https://gitlab.example.com/acme/", "glpat", "")}
	access := accessOf(t, keys, repository, nil)
	if !access.Offered || !strings.Contains(access.Via, "kept token for https://gitlab.example.com/acme") {
		t.Errorf("after keeping one: %+v", access)
	}
	source.credentials = nil
	if accessOf(t, keys, repository, nil).Offered {
		t.Error("a credential was offered after it was removed")
	}
}

// accessOf is how keys reach a repository to read it, failing the test on an error.
func accessOf(t *testing.T, keys *Keys, repository repo.Repo, given []Credential) Access {
	t.Helper()
	access, err := keys.Access(context.Background(), repository, given, Read)
	if err != nil {
		t.Fatal(err)
	}
	return access
}

// fixedMinter makes the same token for every repository it reaches, and remembers what
// each was for.
type fixedMinter struct {
	reaches  bool
	purposes []Purpose
}

func (minter *fixedMinter) Token(_ context.Context, _ repo.Repo, purpose Purpose) (string, error) {
	minter.purposes = append(minter.purposes, purpose)
	if !minter.reaches {
		return "", fmt.Errorf("%w: not installed", ErrUnreachable)
	}
	return "ghs_made", nil
}

func TestAMintedCredentialMakesATokenForEachRepositoryAndPurpose(t *testing.T) {
	keys, err := NewKeys(t.TempDir(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	minter := &fixedMinter{reaches: true}
	source := &changing{credentials: []Credential{KeptMinter("https://github.com/acme", minter, "GitHub App 7")}}
	keys.AlsoKeep(source)
	repository, _ := repo.Of("https://github.com/acme/web")
	access, err := keys.Access(context.Background(), repository, nil, Write)
	if err != nil {
		t.Fatal(err)
	}
	if !access.Offered || !strings.Contains(access.Env[2], base64.StdEncoding.EncodeToString([]byte("x-access-token:ghs_made"))) ||
		access.Via != "kept GitHub App 7 for write" || minter.purposes[0] != Write {
		t.Errorf("access = %+v, purposes %v", access, minter.purposes)
	}
}

func TestARepositoryAMintedCredentialCannotReachIsReadAsNothingWouldAndNeverWritten(t *testing.T) {
	keys, err := NewKeys(t.TempDir(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys.AlsoKeep(&changing{credentials: []Credential{KeptMinter("https://github.com", &fixedMinter{}, "GitHub App 7")}})
	repository, _ := repo.Of("https://github.com/torvalds/linux")
	read, err := keys.Access(context.Background(), repository, nil, Read)
	if err != nil || read.Offered {
		t.Errorf("read = %+v, %v", read, err)
	}
	if _, err := keys.Access(context.Background(), repository, nil, Write); !errors.Is(err, ErrUnreachable) {
		t.Errorf("write = %v", err)
	}
}
