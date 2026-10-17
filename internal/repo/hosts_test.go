package repo

import "testing"

func TestAHostIsReachedOnItsPortsAndAsItsUser(t *testing.T) {
	hosts := Hosts{
		"git.example.com":   {HTTPSPort: 3000, SSHPort: 2222},
		"forge.example.com": {HTTPSPort: 443, SSHPort: 22, SSHUser: "forgejo"},
	}
	gitea := Repo{Host: "git.example.com", Path: "acme/web"}
	forge := Repo{Host: "forge.example.com", Path: "acme/tools/web"}
	github := Repo{Host: "github.com", Path: "oca/web"}
	for got, want := range map[string]string{
		hosts.HTTPS(gitea):     "https://git.example.com:3000/acme/web",
		hosts.SSH(gitea):       "ssh://git@git.example.com:2222/acme/web.git",
		hosts.HTTPS(forge):     "https://forge.example.com/acme/tools/web",
		hosts.SSH(forge):       "forgejo@forge.example.com:acme/tools/web.git",
		hosts.HTTPS(github):    "https://github.com/oca/web",
		hosts.SSH(github):      "git@github.com:oca/web.git",
		Hosts(nil).SSH(github): "git@github.com:oca/web.git",
	} {
		if got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	}
}
