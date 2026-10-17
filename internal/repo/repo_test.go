package repo

import "testing"

func TestEveryShapeOfAnAddressNamesTheSameRepository(t *testing.T) {
	want := Repo{Host: "github.com", Path: "oca/web"}
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

func TestEveryForgeNamesItsRepositoriesTheSameWay(t *testing.T) {
	for address, want := range map[string]Repo{
		"git@gitlab.com:Acme/Tools/Web.git":             {Host: "gitlab.com", Path: "acme/tools/web"},
		"https://gitlab.com/acme/tools/deep/web":        {Host: "gitlab.com", Path: "acme/tools/deep/web"},
		"git@bitbucket.org:acme/web.git":                {Host: "bitbucket.org", Path: "acme/web"},
		"https://codeberg.org/acme/web.git":             {Host: "codeberg.org", Path: "acme/web"},
		"https://git.example.com:3000/acme/web":         {Host: "git.example.com:3000", Path: "acme/web"},
		"git@gitea.example.com:acme/web.git":            {Host: "gitea.example.com", Path: "acme/web"},
		"ssh://git@gitea.example.com:2222/acme/web.git": {Host: "gitea.example.com", Path: "acme/web"},
		"ssh://git@github.com/OCA/web":                  {Host: "github.com", Path: "oca/web"},
		"ssh://forgejo@Git.Example.com/acme/tools/web":  {Host: "git.example.com", Path: "acme/tools/web"},
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
		"ssh://git@-oops/acme/web",
		"ssh://git@github.com/OCA/web?x=1",
		"git://github.com/OCA/web",
		"https://github.com/OCA",
		"https://token@github.com/OCA/web",
		"https://github.com/../web",
		"https://github.com/OCA/..",
		"https://gitlab.com/acme/../../web",
		"https://gitlab.com/acme//web",
		"https://gitlab.com/acme/x.git/web",
		"git@github.com:-oops/web",
		"git@gitlab.com:acme/-oops/web",
		"git@github.com/OCA:web",
	} {
		if got, ok := Of(address); ok {
			t.Errorf("Of(%q) = %v; want no repository", address, got)
		}
	}
}

func TestARelativeSubmoduleIsResolvedAgainstItsSuperproject(t *testing.T) {
	parent := Repo{Host: "github.com", Path: "acme/project"}
	if got := Relative("../web.git", parent); got != "https://github.com/acme/web.git" {
		t.Errorf("Relative = %q", got)
	}
	nested := Repo{Host: "gitlab.com", Path: "acme/tools/project"}
	if got := Relative("../web.git", nested); got != "https://gitlab.com/acme/tools/web.git" {
		t.Errorf("Relative in a group = %q", got)
	}
	if got := Relative("git@github.com:OCA/web.git", parent); got != "git@github.com:OCA/web.git" {
		t.Errorf("an absolute address changed: %q", got)
	}
}

func TestThePoolOfARepositoryStaysUnderThePoolsDirectory(t *testing.T) {
	repository := Repo{Host: "github.com", Path: "oca/web"}
	if got := repository.Dir("/pools"); got != "/pools/github.com/oca/web.git" {
		t.Errorf("Dir = %q", got)
	}
	if got := repository.SSH("git", 0); got != "git@github.com:oca/web.git" {
		t.Errorf("SSH = %q", got)
	}
	gitea := Repo{Host: "git.example.com:3000", Path: "acme/web"}
	if got := gitea.SSH("git", 2222); got != "ssh://git@git.example.com:2222/acme/web.git" {
		t.Errorf("SSH on a port = %q", got)
	}
	if got := gitea.SSH("forgejo", 22); got != "forgejo@git.example.com:acme/web.git" {
		t.Errorf("SSH on 22 = %q", got)
	}
	nested := Repo{Host: "gitlab.com", Path: "acme/tools/web"}
	if got := nested.Dir("/pools"); got != "/pools/gitlab.com/acme/tools/web.git" {
		t.Errorf("Dir = %q", got)
	}
	if nested.Owner() != "acme" || nested.Name() != "web" {
		t.Errorf("Owner, Name = %q, %q", nested.Owner(), nested.Name())
	}
}
