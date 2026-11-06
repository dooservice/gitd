package workspace

import (
	"context"
	"fmt"

	"github.com/dooservice/gitd/internal/git"
)

// startOwnBranch puts a workspace just made on a branch of its own, made at commit: refs
// only, so no file changes. The branch must not exist yet; the workspace keeps following
// the branch it was made from.
func (workspaces *Workspaces) startOwnBranch(ctx context.Context, path, branch, commit string) error {
	for _, command := range []git.Cmd{
		{Dir: path, Args: []string{"update-ref", "--stdin"}, Input: fmt.Sprintf("create refs/heads/%s %s\n", branch, commit)},
		{Dir: path, Args: []string{"symbolic-ref", "HEAD", "refs/heads/" + branch}},
	} {
		if _, err := workspaces.runner.Run(ctx, command); err != nil {
			return fmt.Errorf("starting %s on %s: %w", path, branch, err)
		}
	}
	return nil
}
