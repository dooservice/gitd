package repo

import "testing"

func TestEveryShapeOfAnAddressNamesTheSameRepository(t *testing.T) {
	want := Repo{Host: "github.com", Owner: "oca", Name: "web"}
	for _, address := range []string{
		"https://github.com/OCA/web",
		"https://github.com/OCA/web.git",
		"https://github.com/OCA/web/",
		"git@github.com:OCA/web.git",
		"  git@github.com:oca/web  ",
	} {
		if got, ok := Of(address); !ok || got != want {
			t.Errorf("Of(%q) = %v, %v; want %v", address, got, ok, want)
		}
	}
}

func TestAnAddressThatNamesNoRepositoryIsRefused(t *testing.T) {
	for _, address := range []string{
		"",
		"file:///srv/repos/mine",
		"ext::sh -c touch% /tmp/owned",
		"http://github.com/OCA/web",
		"ssh://git@github.com/OCA/web",
		"https://github.com/OCA",
		"https://github.com/OCA/web/tree/18.0",
		"https://token@github.com/OCA/web",
		"https://github.com/../web",
		"https://github.com/OCA/..",
		"git@github.com:-oops/web",
		"git@github.com/OCA:web",
	} {
		if got, ok := Of(address); ok {
			t.Errorf("Of(%q) = %v; want no repository", address, got)
		}
	}
}

func TestARelativeSubmoduleIsResolvedAgainstItsSuperproject(t *testing.T) {
	parent := Repo{Host: "github.com", Owner: "acme", Name: "project"}
	if got := Relative("../web.git", parent); got != "https://github.com/acme/web.git" {
		t.Errorf("Relative = %q", got)
	}
	if got := Relative("git@github.com:OCA/web.git", parent); got != "git@github.com:OCA/web.git" {
		t.Errorf("an absolute address changed: %q", got)
	}
}

func TestThePoolOfARepositoryStaysUnderThePoolsDirectory(t *testing.T) {
	repository := Repo{Host: "github.com", Owner: "oca", Name: "web"}
	if got := repository.Dir("/pools"); got != "/pools/github.com/oca/web.git" {
		t.Errorf("Dir = %q", got)
	}
	if got := repository.SSH(); got != "git@github.com:oca/web.git" {
		t.Errorf("SSH = %q", got)
	}
}
