// Package repo says which repository an address names.
//
// `git@gitlab.com:Acme/Web.git` and `https://gitlab.com/acme/web` are the same repository,
// so the identity is the host and the path, lowercased. The path is the owner and the name,
// with whatever groups sit between them on a host that nests them (GitLab's subgroups:
// `gitlab.com/acme/tools/web`). An address that does not name a host, an owner and a name
// is not a repository, which is also where `file://`, `ext::` and a pasted web page stop.
//
// SSH or HTTPS is transport, not identity: it follows from the credential the repository is
// read with. So `ssh://git@git.example.com:2222/acme/web.git` is the repository
// `https://git.example.com/acme/web`: the SSH port and user belong to the host, and the
// configuration says them (credentials.Keys).
package repo

import (
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// MaxDepth is how many segments a repository's path may have: GitLab nests up to twenty
// groups, and nothing deeper is a repository.
const MaxDepth = 21

// Repo is one repository, however its address was written. Path is `owner/name`, or
// `owner/group/…/name` on a host with nested groups.
type Repo struct {
	Host string
	Path string
}

// Of returns the repository an address names, and false when it names none.
func Of(address string) (Repo, bool) {
	text := strings.TrimSuffix(strings.TrimSpace(address), "/")
	if strings.HasSuffix(strings.ToLower(text), ".git") {
		text = text[:len(text)-len(".git")]
	}
	host, rest, ok := split(text)
	if !ok || !segment(host) {
		return Repo{}, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || len(parts) > MaxDepth {
		return Repo{}, false
	}
	for index, part := range parts {
		// A group named `x.git` would read as a pool on disk, so only the last part may
		// have ended in `.git`, and that was trimmed above.
		if !segment(part) || (index < len(parts)-1 && strings.HasSuffix(strings.ToLower(part), ".git")) {
			return Repo{}, false
		}
	}
	return Repo{Host: strings.ToLower(host), Path: strings.ToLower(rest)}, true
}

// segment is a part a pool's path can be built from: one that does not climb out of the
// pools directory, nor reads as an option to git.
func segment(part string) bool {
	return part != "" && part != "." && part != ".." && !strings.HasPrefix(part, "-") && !strings.ContainsAny(part, `\`)
}

func split(text string) (host, rest string, ok bool) {
	// git's scp-like form, `user@host:path`: no `://`, and the `:` comes before any `/`.
	if !strings.Contains(text, "://") {
		_, after, found := strings.Cut(text, "@")
		if !found {
			return "", "", false
		}
		host, rest, ok = strings.Cut(after, ":")
		return host, rest, ok && !strings.Contains(host, "/")
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", false
	}
	switch {
	case parsed.Scheme == "https" && parsed.User == nil:
		return parsed.Host, strings.TrimPrefix(parsed.Path, "/"), true
	case parsed.Scheme == "ssh":
		// The user and the port are the host's way of taking SSH, not part of the name.
		return parsed.Hostname(), strings.TrimPrefix(parsed.Path, "/"), true
	}
	return "", "", false
}

// Relative resolves a submodule address written relative to its superproject, as
// `../web.git`, and returns any other address unchanged.
func Relative(address string, parent Repo) string {
	if !strings.HasPrefix(address, "./") && !strings.HasPrefix(address, "../") {
		return address
	}
	return "https://" + parent.Host + path.Join("/", parent.Path, address)
}

// HTTPS is the canonical address, and the one stored and compared.
func (repository Repo) HTTPS() string {
	return "https://" + repository.Host + "/" + repository.Path
}

// SSH is the address a deploy key reads the repository at, as user on port; a port of 0
// is SSH's own, 22. A port in the host is the HTTPS one, so it is left out.
func (repository Repo) SSH(user string, port int) string {
	host, _, _ := strings.Cut(repository.Host, ":")
	if port != 0 && port != 22 {
		return "ssh://" + user + "@" + host + ":" + strconv.Itoa(port) + "/" + repository.Path + ".git"
	}
	return user + "@" + host + ":" + repository.Path + ".git"
}

// SSHHost is the host SSH is read from: the host without an HTTPS port.
func (repository Repo) SSHHost() string {
	host, _, _ := strings.Cut(repository.Host, ":")
	return host
}

// Dir is where the repository's pool lives under the pools directory.
func (repository Repo) Dir(pools string) string {
	return filepath.Join(pools, repository.Host, filepath.FromSlash(repository.Path)+".git")
}

func (repository Repo) String() string {
	return repository.HTTPS()
}
