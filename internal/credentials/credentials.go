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
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

// Credential opens the repositories whose canonical address starts with Prefix, with a
// token or with the path of a key, whichever it carries.
type Credential struct {
	Prefix string
	Token  string
	Key    string
}

// FromProto reads the credentials of a request, each prefix in canonical form.
func FromProto(given []*gitdv1.Credential) []Credential {
	read := make([]Credential, 0, len(given))
	for _, credential := range given {
		read = append(read, Credential{
			Prefix: canonicalPrefix(credential.GetUrlPrefix()),
			Token:  credential.GetHttpsToken(),
			Key:    credential.GetSshKeyPath(),
		})
	}
	return read
}

// canonicalPrefix writes a prefix the way a canonical address starts, so that
// `git@github.com:acme/` and `https://github.com/acme/` open the same repositories.
func canonicalPrefix(prefix string) string {
	text := strings.ToLower(strings.TrimSpace(prefix))
	if after, ok := strings.CutPrefix(text, "git@"); ok {
		host, path, _ := strings.Cut(after, ":")
		text = "https://" + host + "/" + path
	}
	return strings.TrimSuffix(strings.TrimSuffix(text, "/"), ".git")
}

func (credential Credential) opens(repository repo.Repo) bool {
	address := repository.HTTPS()
	return credential.Prefix == "" || address == credential.Prefix || strings.HasPrefix(address, credential.Prefix+"/")
}

func (credential Credential) carries() bool {
	return credential.Token != "" || credential.Key != ""
}
