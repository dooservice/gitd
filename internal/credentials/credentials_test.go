package credentials

import (
	"encoding/base64"
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
	} {
		if got := canonicalPrefix(prefix); got != want {
			t.Errorf("canonicalPrefix(%q) = %q", prefix, got)
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
	access := keys.For(repository, given)
	if access.URL != "git@github.com:acme/web.git" || !strings.Contains(access.Env[0], "'/keys/web'") {
		t.Errorf("own key = %+v", access)
	}
	public, _ := repo.Of("git@github.com:OCA/web.git")
	if access := keys.For(public, given); !strings.Contains(access.Env[0], "'/keys/project'") {
		t.Errorf("project key = %+v", access)
	}
	private, _ := repo.Of("git@github.com:other/web.git")
	if access := keys.For(private, given); access.URL != "https://github.com/other/web" || !access.Offered {
		t.Errorf("token = %+v", access)
	}
	if access := keys.For(public, nil); access.Offered || access.Env != nil {
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
	keys, err := NewKeys(filepath.Join(dir, "state"), []string{extra}, nil)
	if err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(keys.knownHosts)
	for _, host := range []string{"github.com ", "gitlab.com ", "bitbucket.org ", "codeberg.org ", "git.example.com "} {
		if !strings.Contains(string(written), "\n"+host) && !strings.HasPrefix(string(written), host) {
			t.Errorf("no key for %s", host)
		}
	}
	if _, err := NewKeys(filepath.Join(dir, "state"), []string{filepath.Join(dir, "missing")}, nil); err == nil {
		t.Error("a missing known hosts file was not refused")
	}
}

func TestAKeyReadsOnThePortAndAsTheUserItsHostTakesSSHWith(t *testing.T) {
	keys, err := NewKeys(t.TempDir(), nil, map[string]config.SSHHost{
		"Git.Example.com":   {Port: 2222},
		"forge.example.com": {User: "forgejo"},
	})
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
		access := keys.For(repository, []Credential{{Key: "/keys/web"}})
		if access.URL != want {
			t.Errorf("%s: URL = %q; want %q", address, access.URL, want)
		}
	}
}
