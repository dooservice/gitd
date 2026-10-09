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

func TestTheMostSpecificCredentialIsTriedFirstAndNothingLast(t *testing.T) {
	keys := &Keys{knownHosts: "/state/known_hosts"}
	repository, _ := repo.Of("git@github.com:acme/web.git")
	given := []Credential{
		{Prefix: "https://github.com", Token: "account"},
		{Prefix: "https://github.com/acme/web", Key: "/keys/web"},
		{Prefix: "https://github.com/other", Token: "elsewhere"},
	}
	attempts := keys.Attempts(repository, given, false)
	if len(attempts) != 3 {
		t.Fatalf("attempts = %d", len(attempts))
	}
	if attempts[0].URL != "git@github.com:acme/web.git" || !strings.Contains(attempts[0].Env[0], "'/keys/web'") {
		t.Errorf("first = %+v", attempts[0])
	}
	if attempts[1].URL != "https://github.com/acme/web" || !attempts[1].Offered {
		t.Errorf("second = %+v", attempts[1])
	}
	if attempts[2].Offered || attempts[2].Env != nil {
		t.Errorf("last = %+v", attempts[2])
	}
	if first := keys.Attempts(repository, given, true)[0]; first.Offered {
		t.Errorf("anonymous was not first: %+v", first)
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
