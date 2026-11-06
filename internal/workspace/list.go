package workspace

import (
	"context"
	"runtime"
	"sync"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

// listWorkers is how many workspaces List describes at once: each description waits on a
// few git processes, which run better side by side.
var listWorkers = min(4*runtime.NumCPU(), 16)

// List is every workspace gitd made that is still on disk, each with what Describe says of
// it and the branches published from it, in the order of their paths. A workspace that
// cannot be described right now, as one gitd refuses to run git in, is listed by its path
// alone.
func (workspaces *Workspaces) List(ctx context.Context) (*gitdv1.ListResponse, error) {
	var paths []string
	for _, path := range workspaces.pools.Owned() {
		if _, err := workspaces.allowed(path); err == nil {
			paths = append(paths, path)
		}
	}
	listed := make([]*gitdv1.ListedWorkspace, len(paths))
	slots := make(chan struct{}, listWorkers)
	var described sync.WaitGroup
	for index, path := range paths {
		described.Add(1)
		slots <- struct{}{}
		go func() {
			defer described.Done()
			defer func() { <-slots }()
			listed[index] = workspaces.listed(ctx, path)
		}()
	}
	described.Wait()
	return &gitdv1.ListResponse{Workspaces: listed}, ctx.Err()
}

// listed is one workspace as List answers it; the repository it names is taken as its own
// only when its pool says so.
func (workspaces *Workspaces) listed(ctx context.Context, path string) *gitdv1.ListedWorkspace {
	entry := &gitdv1.ListedWorkspace{Path: path}
	described, err := workspaces.Describe(ctx, &gitdv1.DescribeRequest{Path: path})
	if err != nil {
		return entry
	}
	entry.Description = described
	if repository, ok := repo.Of(described.GetUrl()); ok && workspaces.pools.Owns(repository, path) {
		entry.Published = workspaces.pools.Published(repository, path)
	}
	return entry
}
