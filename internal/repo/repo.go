// Package repo says which repository an address names.
//
// `git@github.com:OCA/web.git` and `https://github.com/OCA/web` are the same repository, so
// the identity is the host, the owner and the name, lowercased. It is the same rule as
// `dooservice_git.Repo`: an address that does not name all three is not a repository, which
// is also where `file://`, `ext::` and a pasted web page stop.
//
// SSH or HTTPS is transport, not identity: it follows from the credential the repository is
// read with.
package repo

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// Repo is one repository, however its address was written.
type Repo struct {
	Host  string
	Owner string
	Name  string
}

// Of returns the repository an address names, and false when it names none.
func Of(address string) (Repo, bool) {
	text := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(address), "/"), ".git")
	host, rest, ok := split(text)
	if !ok {
		return Repo{}, false
	}
	owner, name, ok := strings.Cut(rest, "/")
	if !ok || !segment(host) || !segment(owner) || !segment(name) || strings.Contains(name, "/") {
		return Repo{}, false
	}
	return Repo{Host: strings.ToLower(host), Owner: strings.ToLower(owner), Name: strings.ToLower(name)}, true
}

// segment is a part a pool's path can be built from: one that does not climb out of the
// pools directory, nor reads as an option to git.
func segment(part string) bool {
	return part != "" && part != "." && part != ".." && !strings.HasPrefix(part, "-")
}

func split(text string) (host, rest string, ok bool) {
	if after, found := strings.CutPrefix(text, "git@"); found {
		host, rest, ok = strings.Cut(after, ":")
		return host, rest, ok && !strings.Contains(host, "/")
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", false
	}
	return parsed.Host, strings.TrimPrefix(parsed.Path, "/"), true
}

// Relative resolves a submodule address written relative to its superproject, as
// `../web.git`, and returns any other address unchanged.
func Relative(address string, parent Repo) string {
	if !strings.HasPrefix(address, "./") && !strings.HasPrefix(address, "../") {
		return address
	}
	return "https://" + parent.Host + path.Join("/", parent.Owner, parent.Name, address)
}

// HTTPS is the canonical address, and the one stored and compared.
func (repository Repo) HTTPS() string {
	return "https://" + repository.Host + "/" + repository.Owner + "/" + repository.Name
}

// SSH is the address a deploy key reads the repository at.
func (repository Repo) SSH() string {
	return "git@" + repository.Host + ":" + repository.Owner + "/" + repository.Name + ".git"
}

// Dir is where the repository's pool lives under the pools directory.
func (repository Repo) Dir(pools string) string {
	return filepath.Join(pools, repository.Host, repository.Owner, repository.Name+".git")
}

func (repository Repo) String() string {
	return repository.HTTPS()
}
