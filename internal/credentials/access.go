package credentials

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/config"
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
// configuration says otherwise.
type Keys struct {
	knownHosts string
	hosts      map[string]config.SSHHost
}

// NewKeys writes the host keys under dir, where every ssh gitd starts reads them: the
// built-in ones followed by those of each file in extra. hosts are the hosts that take
// SSH on another port or as another user.
func NewKeys(dir string, extra []string, hosts map[string]config.SSHHost) (*Keys, error) {
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
	lowered := make(map[string]config.SSHHost, len(hosts))
	for host, ssh := range hosts {
		lowered[strings.ToLower(host)] = ssh
	}
	return &Keys{knownHosts: path, hosts: lowered}, nil
}

// sshURL is the address a key reads the repository at, on the port and as the user its
// host takes SSH with.
func (keys *Keys) sshURL(repository repo.Repo) string {
	ssh := keys.hosts[repository.SSHHost()]
	user := ssh.User
	if user == "" {
		user = "git"
	}
	return repository.SSH(user, ssh.Port)
}

// For is how a repository is read: with the most specific credential that opens it, or
// with nothing when none does. One credential and one attempt — which one opens a
// repository is decided by whoever sends the request, from what its host says about it,
// so nothing here guesses by trying them in turn.
func (keys *Keys) For(repository repo.Repo, given []Credential) Access {
	var chosen *Credential
	for index, credential := range given {
		if !credential.carries() || !credential.opens(repository) {
			continue
		}
		if chosen == nil || len(credential.Prefix) > len(chosen.Prefix) {
			chosen = &given[index]
		}
	}
	if chosen == nil {
		return Access{URL: repository.HTTPS(), Via: "nothing"}
	}
	return keys.access(repository, *chosen)
}

func (keys *Keys) access(repository repo.Repo, credential Credential) Access {
	if credential.Token != "" {
		user := credential.User
		if user == "" {
			user = TokenUser(repository.Host)
		}
		basic := base64.StdEncoding.EncodeToString([]byte(user + ":" + credential.Token))
		return Access{
			URL: repository.HTTPS(),
			Env: []string{
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=http." + repository.HTTPS() + ".extraHeader",
				"GIT_CONFIG_VALUE_0=Authorization: Basic " + basic,
			},
			Offered: true,
			Via:     "token for " + credential.Prefix,
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
		URL:     keys.sshURL(repository),
		Env:     []string{"GIT_SSH_COMMAND=" + strings.Join(ssh, " ")},
		Offered: true,
		Via:     "key " + credential.Key,
	}
}

// quoted makes a path one word for the shell git runs `GIT_SSH_COMMAND` with.
func quoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
