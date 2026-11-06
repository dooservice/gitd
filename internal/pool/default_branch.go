package pool

import (
	"sync"

	"github.com/dooservice/gitd/internal/repo"
)

// defaultBranches remembers each pool's default branch as git last said it, so a workspace
// with no branch asked for does not start a git to read it again. A pool's HEAD changes only
// when it is fetched or made, and Sync forgets the branch first.
type defaultBranches struct {
	guard  sync.Mutex
	branch map[repo.Repo]string
}

func newDefaultBranches() *defaultBranches {
	return &defaultBranches{branch: map[repo.Repo]string{}}
}

func (defaults *defaultBranches) get(repository repo.Repo) (string, bool) {
	defaults.guard.Lock()
	defer defaults.guard.Unlock()
	branch, ok := defaults.branch[repository]
	return branch, ok
}

func (defaults *defaultBranches) set(repository repo.Repo, branch string) {
	defaults.guard.Lock()
	defer defaults.guard.Unlock()
	defaults.branch[repository] = branch
}

func (defaults *defaultBranches) forget(repository repo.Repo) {
	defaults.guard.Lock()
	defer defaults.guard.Unlock()
	delete(defaults.branch, repository)
}
