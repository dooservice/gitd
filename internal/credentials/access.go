package credentials

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/repo"
)

//go:embed known_hosts
var builtInKnownHosts []byte

// Access is one way of reading a repository: the address, and what goes with it in git's
// environment. Offered says whether a credential goes with it, which is what tells a
// refused one from none at all; Via names it for the logs, never its secret.
type Access struct {
	URL     string
	Env     []string
	Offered bool
	Via     string
}

// Keys hands ssh the host keys every connection is checked against, and says how each
// host takes SSH. Nothing is trusted on first use: the keys are the ones GitHub, GitLab,
// Bitbucket and Codeberg publish, built into the binary, and those of the files the
// configuration names, for any other host. A host takes SSH as `git` on port 22 unless the
// configuration says otherwise (repo.Hosts).
type Keys struct {
	knownHosts string
	hosts      repo.Hosts
	stored     []Credential
}

// NewKeys writes the host keys under dir, where every ssh gitd starts reads them: the
// built-in ones followed by those of each file in extra. hosts say how the hosts not
// reached the usual way are, lowercased as config.Load leaves them, and stored are the
// credentials the configuration keeps (Stored).
func NewKeys(dir string, extra []string, hosts repo.Hosts, stored []Credential) (*Keys, error) {
	keys := append([]byte(nil), builtInKnownHosts...)
	for _, file := range extra {
		read, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading known hosts: %w", err)
		}
		if len(keys) > 0 && keys[len(keys)-1] != '\n' {
			keys = append(keys, '\n')
		}
		keys = append(keys, read...)
	}
	path := filepath.Join(dir, "known_hosts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, keys, 0o644); err != nil {
		return nil, err
	}
	return &Keys{knownHosts: path, hosts: hosts, stored: stored}, nil
}

// Address is where a repository is read over HTTPS, on its host's port: what a copy of it
// names as its origin.
func (keys *Keys) Address(repository repo.Repo) string {
	return keys.hosts.HTTPS(repository)
}

// For is how a repository is read: with the most specific credential that opens it, of
// those the request gives and those gitd keeps, or with nothing when none does. A request's
// wins over a kept one of the same prefix, so a caller can always say which. One credential
// and one attempt: nothing here guesses by trying them in turn.
func (keys *Keys) For(repository repo.Repo, given []Credential) Access {
	candidates := append(append([]Credential(nil), given...), keys.stored...)
	var chosen *Credential
	for index, credential := range candidates {
		if !credential.carries() || !credential.opens(repository) {
			continue
		}
		if chosen == nil || len(credential.Prefix) > len(chosen.Prefix) {
			chosen = &candidates[index]
		}
	}
	if chosen == nil {
		return Access{URL: keys.Address(repository), Via: "nothing"}
	}
	return keys.access(repository, *chosen)
}

func (keys *Keys) access(repository repo.Repo, credential Credential) Access {
	if credential.Token != "" {
		user := credential.User
		if user == "" {
			user = tokenUser(repository.Host)
		}
		basic := base64.StdEncoding.EncodeToString([]byte(user + ":" + credential.Token))
		address := keys.Address(repository)
		return Access{
			URL: address,
			Env: []string{
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=http." + address + ".extraHeader",
				"GIT_CONFIG_VALUE_0=Authorization: Basic " + basic,
			},
			Offered: true,
			Via:     credential.kept() + "token for " + credential.Prefix,
		}
	}
	ssh := []string{
		"ssh", "-F", "/dev/null",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + quoted(keys.knownHosts),
		"-o", "IdentitiesOnly=yes",
		"-i", quoted(credential.Key),
	}
	return Access{
		URL:     keys.hosts.SSH(repository),
		Env:     []string{"GIT_SSH_COMMAND=" + strings.Join(ssh, " ")},
		Offered: true,
		Via:     credential.kept() + "key " + credential.Key,
	}
}

// quoted makes a path one word for the shell git runs `GIT_SSH_COMMAND` with.
func quoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
