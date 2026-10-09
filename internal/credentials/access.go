package credentials

import (
	_ "embed"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/repo"
)

//go:embed github_known_hosts
var githubKnownHosts []byte

// Access is one way of reading a repository: the address, and what goes with it in git's
// environment. Offered says whether a credential goes with it, which is what tells a
// refused one from none at all; Via names it for the logs, never its secret.
type Access struct {
	URL     string
	Env     []string
	Offered bool
	Via     string
}

// Keys hands ssh the host keys every connection is checked against. Nothing is trusted on
// first use: they are GitHub's published ones, built into the binary.
type Keys struct {
	knownHosts string
}

// NewKeys writes the host keys under dir, where every ssh gitd starts reads them.
func NewKeys(dir string) (*Keys, error) {
	path := filepath.Join(dir, "known_hosts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, githubKnownHosts, 0o644); err != nil {
		return nil, err
	}
	return &Keys{knownHosts: path}, nil
}

// For is how a repository is read: with the most specific credential that opens it, or
// with nothing when none does. One credential and one attempt — which one opens a
// repository is decided by whoever sends the request, from what GitHub says about it,
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
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + credential.Token))
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
		URL:     repository.SSH(),
		Env:     []string{"GIT_SSH_COMMAND=" + strings.Join(ssh, " ")},
		Offered: true,
		Via:     "key " + credential.Key,
	}
}

// quoted makes a path one word for the shell git runs `GIT_SSH_COMMAND` with.
func quoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
