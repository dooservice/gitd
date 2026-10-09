package credentials

import (
	"cmp"
	_ "embed"
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
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

// Attempts are the ways of reading a repository with the credentials given, the most
// specific first. Reading with nothing goes first when anonymousFirst is set, and last
// otherwise.
func (keys *Keys) Attempts(repository repo.Repo, given []Credential, anonymousFirst bool) []Access {
	matching := slices.DeleteFunc(slices.Clone(given), func(credential Credential) bool {
		return !credential.carries() || !credential.opens(repository)
	})
	slices.SortStableFunc(matching, func(one, other Credential) int {
		return cmp.Compare(len(other.Prefix), len(one.Prefix))
	})
	attempts := make([]Access, 0, len(matching)+1)
	for _, credential := range matching {
		attempts = append(attempts, keys.access(repository, credential))
	}
	anonymous := Access{URL: repository.HTTPS(), Via: "nothing"}
	if anonymousFirst {
		return append([]Access{anonymous}, attempts...)
	}
	return append(attempts, anonymous)
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
