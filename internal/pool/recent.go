package pool

import (
	"sync"
	"time"

	"github.com/dooservice/gitd/internal/repo"
)

// recentSyncs remembers when each branch of each pool was last brought in, and takes one
// brought in within the last moments as current.
type recentSyncs struct {
	guard  sync.Mutex
	synced map[recentKey]time.Time
	within time.Duration
}

type recentKey struct {
	repository repo.Repo
	branch     string
}

func newRecentSyncs(within time.Duration) *recentSyncs {
	return &recentSyncs{synced: map[recentKey]time.Time{}, within: within}
}

func (recent *recentSyncs) current(repository repo.Repo, branch string) bool {
	recent.guard.Lock()
	defer recent.guard.Unlock()
	return time.Since(recent.synced[recentKey{repository, branch}]) < recent.within
}

func (recent *recentSyncs) note(repository repo.Repo, branches ...string) {
	recent.guard.Lock()
	defer recent.guard.Unlock()
	for _, branch := range branches {
		recent.synced[recentKey{repository, branch}] = time.Now()
	}
}
