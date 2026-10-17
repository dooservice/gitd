package workspace

import (
	"log/slog"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// Publishing is the rules a publish follows: Settings, when set, say per repository the
// branch prefix, the paths a publish may not change or delete and how many files it may
// change; Recorder, when set, keeps every publish, done or refused.
type Publishing struct {
	Settings Settings
	Recorder Recorder
}

// Recorder keeps a record of a publish: the audit log.
type Recorder interface {
	Record(entry *gitdv1.AuditEntry) error
}

// UsePublishing has publishes follow rules.
func (workspaces *Workspaces) UsePublishing(rules Publishing) {
	workspaces.publishing = rules
}

// branchAllowed refuses a branch outside the prefix publishing is limited to.
func branchAllowed(branch, prefix string) error {
	if prefix == "" || strings.HasPrefix(branch, prefix) {
		return nil
	}
	return git.Fail(git.BranchRefused, "%q does not start with %q: gitd publishes only to branches that do", branch, prefix)
}

// record keeps what a publish did, or why it was refused. A publish is done by the time it
// is recorded, so an entry that cannot be written is logged as an error, not answered as one.
func (workspaces *Workspaces) record(entry *gitdv1.AuditEntry, answer *gitdv1.PublishResponse, err error) {
	recorder := workspaces.publishing.Recorder
	if recorder == nil {
		return
	}
	if err != nil {
		failure, ok := git.AsFailure(err)
		if !ok {
			failure = git.Fail(git.Failed, "%s", err)
		}
		entry.Refusal = failure.Proto()
	} else {
		entry.Repository, entry.Submodules = answer.GetRepository(), answer.GetSubmodules()
	}
	if err := recorder.Record(entry); err != nil {
		slog.Error("a publish is missing from the audit log", "workspace", entry.GetWorkspace(),
			"branch", entry.GetBranch(), "error", err)
	}
}
