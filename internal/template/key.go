// Package template keeps the templates copy-on-write workspaces are made from: one
// read-only checkout of a repository at a commit, built once and shared by every workspace
// of that commit, and retired once none uses it.
//
// A template is what a workspace is today — a checkout whose `.git` borrows from the pool
// — and it never changes once built: a layer on top of it must never see it move. Building
// it is the workspace package's (Builder); keeping, sharing and retiring it is this one's.
package template

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/dooservice/gitd/internal/repo"
)

// fullCommit is a whole object name, SHA-1 or SHA-256: a template is of one commit, never
// of a branch or an abbreviation, which could later name another.
var fullCommit = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// Key is which template: a repository at a commit, with its submodules checked out or not.
type Key struct {
	Repository repo.Repo
	Commit     string
	Submodules bool
}

// Valid says why a key cannot name a template, or nothing when it can.
func (key Key) Valid() error {
	if key.Repository.Host == "" || key.Repository.Path == "" {
		return fmt.Errorf("a template needs a repository")
	}
	if !fullCommit.MatchString(key.Commit) {
		return fmt.Errorf("a template is of a whole commit, not %q", key.Commit)
	}
	return nil
}

// Dir is where the template lives under the templates directory:
// `<host>/<path>/<commit>`, and `<commit>+submodules` with its submodules checked out.
func (key Key) Dir(templates string) string {
	name := key.Commit
	if key.Submodules {
		name += "+submodules"
	}
	return filepath.Join(templates, key.Repository.Host, filepath.FromSlash(key.Repository.Path), name)
}

func (key Key) String() string {
	name := key.Repository.HTTPS() + "@" + key.Commit
	if key.Submodules {
		name += "+submodules"
	}
	return name
}
