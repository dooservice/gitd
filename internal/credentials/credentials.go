// Package credentials turns the credentials of one request into how git reads a repository.
//
// A credential never reaches the command line or a file: a token travels as an HTTP header
// in `GIT_CONFIG_*`, scoped to the one repository being read, and a key is the path of one
// already on this server, handed to ssh through `GIT_SSH_COMMAND`.
//
// The transport follows from the credential: a token reads over HTTPS and a key over SSH,
// whichever way the address was written. So a private repository somebody is only a
// collaborator on, which takes no deploy key of theirs, is read with the account's token
// even when its `.gitmodules` line says `git@`.
package credentials

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/repo"
)

// Credential opens the repositories whose canonical address starts with Prefix, with a
// token or with the path of a key, whichever it carries. User goes with the token, and
// empty is the one its host expects (see tokenUser).
type Credential struct {
	Prefix string
	Token  string
	User   string
	Key    string
	stored bool
}

// FromProto reads the credentials of a request, each prefix in canonical form.
func FromProto(given []*gitdv1.Credential) []Credential {
	read := make([]Credential, 0, len(given))
	for _, credential := range given {
		read = append(read, Credential{
			Prefix: canonicalPrefix(credential.GetUrlPrefix()),
			Token:  credential.GetHttpsToken(),
			User:   credential.GetHttpsUser(),
			Key:    credential.GetSshKeyPath(),
		})
	}
	return read
}

// Stored reads the credentials the configuration keeps, each secret from where it says:
// a token from its file or its environment variable, a key by its path, which ssh reads.
// A secret that cannot be read stops the start, rather than leaving its repositories to
// fail one by one.
func Stored(settings []config.Credential) ([]Credential, error) {
	stored := make([]Credential, 0, len(settings))
	for _, setting := range settings {
		credential := Credential{Prefix: canonicalPrefix(setting.Prefix), User: setting.TokenUser, Key: setting.SSHKey, stored: true}
		switch {
		case setting.TokenFile != "":
			read, err := os.ReadFile(setting.TokenFile)
			if err != nil {
				return nil, fmt.Errorf("the token for %q: %w", setting.Prefix, err)
			}
			credential.Token = strings.TrimSpace(string(read))
		case setting.TokenEnv != "":
			credential.Token = strings.TrimSpace(os.Getenv(setting.TokenEnv))
			if credential.Token == "" {
				return nil, fmt.Errorf("the token for %q: $%s is empty", setting.Prefix, setting.TokenEnv)
			}
		default:
			if _, err := os.Stat(setting.SSHKey); err != nil {
				return nil, fmt.Errorf("the key for %q: %w", setting.Prefix, err)
			}
		}
		stored = append(stored, credential)
	}
	return stored, nil
}

// canonicalPrefix writes a prefix the way a canonical address starts, without a port, so
// that `git@host:acme/`, `ssh://git@host:2222/acme/` and `https://host:3000/acme/` open the
// same repositories.
func canonicalPrefix(prefix string) string {
	text := strings.ToLower(strings.TrimSpace(prefix))
	if !strings.Contains(text, "://") {
		if _, after, ok := strings.Cut(text, "@"); ok {
			host, path, _ := strings.Cut(after, ":")
			text = "https://" + host + "/" + path
		}
	}
	if parsed, err := url.Parse(text); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "ssh") {
		text = "https://" + parsed.Hostname() + parsed.Path
	}
	return strings.TrimSuffix(strings.TrimSuffix(text, "/"), ".git")
}

func (credential Credential) opens(repository repo.Repo) bool {
	address := repository.HTTPS()
	return credential.Prefix == "" || address == credential.Prefix || strings.HasPrefix(address, credential.Prefix+"/")
}

// tokenUser is the user name a token is sent with to a host that was given none: what
// gitlab.com and bitbucket.org expect for theirs, and GitHub's `x-access-token` anywhere
// else, which Gitea and Forgejo accept as they take any name.
func tokenUser(host string) string {
	switch strings.ToLower(host) {
	case "gitlab.com":
		return "oauth2"
	case "bitbucket.org":
		return "x-token-auth"
	}
	return "x-access-token"
}

func (credential Credential) carries() bool {
	return credential.Token != "" || credential.Key != ""
}

// kept says, in the logs, that a credential is one gitd keeps rather than the request's.
func (credential Credential) kept() string {
	if credential.stored {
		return "kept "
	}
	return ""
}
