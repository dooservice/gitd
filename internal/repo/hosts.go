package repo

import (
	"cmp"
	"strconv"
)

// Host is how a host is reached when not over HTTPS on 443 and SSH as `git` on 22, as a
// Gitea or Forgejo on `:3000` and `:2222` is. A port of 0 is the usual one.
type Host struct {
	HTTPSPort int    `toml:"https_port"`
	SSHPort   int    `toml:"ssh_port"`
	SSHUser   string `toml:"ssh_user"`
}

// Hosts are the hosts reached otherwise, by host name as repositories name them
// (lowercased). A host that is not here is reached the usual way.
type Hosts map[string]Host

// HTTPS is the address a repository is read at over HTTPS, on its host's port.
func (hosts Hosts) HTTPS(repository Repo) string {
	address := repository.Host
	if port := hosts[repository.Host].HTTPSPort; port != 0 && port != 443 {
		address += ":" + strconv.Itoa(port)
	}
	return "https://" + address + "/" + repository.Path
}

// SSH is the address a key reads a repository at, as its host's user and on its port.
func (hosts Hosts) SSH(repository Repo) string {
	host := hosts[repository.Host]
	user := cmp.Or(host.SSHUser, "git")
	if host.SSHPort != 0 && host.SSHPort != 22 {
		return "ssh://" + user + "@" + repository.Host + ":" + strconv.Itoa(host.SSHPort) + "/" + repository.Path + ".git"
	}
	return user + "@" + repository.Host + ":" + repository.Path + ".git"
}
