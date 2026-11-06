package workspace

import (
	"context"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// List is every workspace gitd made that is still on disk, each with what Describe says of
// it and the branches published from it. A workspace that cannot be described right now,
// as one gitd refuses to run git in, is listed by its path alone.
func (workspaces *Workspaces) List(ctx context.Context) (*gitdv1.ListResponse, error) {
	listed := &gitdv1.ListResponse{}
	for _, path := range workspaces.pools.Owned() {
		if _, err := workspaces.allowed(path); err != nil {
			continue
		}
		entry := &gitdv1.ListedWorkspace{Path: path}
		if described, err := workspaces.Describe(ctx, &gitdv1.DescribeRequest{Path: path}); err == nil {
			entry.Description = described
		}
		if repository, err := workspaces.madeFor(ctx, path); err == nil && repository != nil {
			entry.Published = workspaces.pools.Published(*repository, path)
		}
		listed.Workspaces = append(listed.Workspaces, entry)
	}
	return listed, nil
}
