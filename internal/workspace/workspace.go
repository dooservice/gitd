// Package workspace checks repositories out where their users need them.
//
// A workspace is an ordinary clone whose objects live in its repository's pool, through
// `objects/info/alternates`: it takes little disk and seconds to make. Its `origin` is the
// repository's real address, so whoever runs git in it sees what a clone shows, but it never reaches
// the network itself: the pool is synced, and the workspace then fetches from the pool.
//
// A workspace is built next to where it goes and renamed into place when it is complete,
// so a crash leaves at most a temporary directory that the next start removes. What is at
// the destination is never replaced unless gitd made it.
package workspace

import (
	"path/filepath"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// Report hands progress to whoever asked, naming the repository it is about.
type Report func(*gitdv1.Progress)

// Workspaces are the workspaces under the roots gitd may write in. Writes bounds how many
// operations run at once, and a git `.lock` older than staleLock is taken as left behind.
type Workspaces struct {
	roots     []string
	runner    *git.Runner
	pools     *pool.Pools
	locks     *lock.Locks
	writes    *lock.Queue
	staleLock time.Duration
}

// New returns the workspaces under roots. A root is kept with its symbolic links resolved,
// as every path asked for is compared once its own are.
func New(
	roots []string, runner *git.Runner, pools *pool.Pools, locks *lock.Locks, writes *lock.Queue, staleLock time.Duration,
) *Workspaces {
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
		resolved = append(resolved, filepath.Clean(root))
	}
	return &Workspaces{roots: resolved, runner: runner, pools: pools, locks: locks, writes: writes, staleLock: staleLock}
}

// Result is where a workspace ended up, and the submodules that could not come along.
type Result struct {
	Commit  string
	Branch  string
	Pending []*gitdv1.PendingSubmodule
}

// Proto is the result as the contract carries it.
func (result Result) Proto() *gitdv1.WorkspaceResult {
	return &gitdv1.WorkspaceResult{Commit: result.Commit, Branch: result.Branch, PendingSubmodules: result.Pending}
}

// job is one write on one workspace: where it is, the repository it holds, the revision it
// goes to, whether its submodules come too, whether it keeps the repository's whole
// history, and the credentials it may read with.
type job struct {
	path       string
	repository repo.Repo
	revision   *gitdv1.Revision
	submodules bool
	full       bool
	given      []credentials.Credential
	report     Report
}

// want is what the job needs of its pool: the commit it goes to, the whole history, or the
// tip of its branch.
func (work job) want() pool.Want {
	if commit := work.revision.GetCommit(); commit != "" {
		return pool.OnlyCommit(commit)
	}
	if work.full {
		return pool.Everything()
	}
	return pool.OnlyBranch(work.revision.GetBranch())
}

func onBranch(branch string) *gitdv1.Revision {
	return &gitdv1.Revision{Target: &gitdv1.Revision_Branch{Branch: branch}}
}

func atCommit(commit string) *gitdv1.Revision {
	return &gitdv1.Revision{Target: &gitdv1.Revision_Commit{Commit: commit}}
}

// reporter turns git's progress into the contract's, naming the repository.
func reporter(report Report, repository repo.Repo) func(string, int) {
	if report == nil {
		return nil
	}
	return func(phase string, percent int) {
		report(&gitdv1.Progress{Phase: phase, Percent: int32(percent), Repository: repository.HTTPS()})
	}
}
