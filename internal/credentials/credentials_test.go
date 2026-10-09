package credentials

import (
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/repo"
)

func TestThePrefixOfAKeyAndOfATokenAreWrittenTheSameWay(t *testing.T) {
	for prefix, want := range map[string]string{
		"git@github.com:Acme/":          "https://github.com/acme",
		"https://github.com/acme/":      "https://github.com/acme",
		"https://github.com/acme/x.git": "https://github.com/acme/x",
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
