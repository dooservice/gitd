package workspace

import (
	"context"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// gitCandidates are the paths that may have changed in a workspace without a layer of its
// own, as git lists them there: what differs from its HEAD, committed or untracked, and
// what its commits changed since the commit gitd last put it on — or since its HEAD, when
// gitd noted none. Git runs in the workspace only after the checks of its .git every call
// that runs git there makes.
func (workspaces *Workspaces) gitCandidates(
	ctx context.Context, repository repo.Repo, path string, includeIgnored bool,
) ([]string, string, error) {
	if err := workspaces.guard(ctx, path); err != nil {
		return nil, "", err
	}
	base, noted := workspaces.pools.Base(repository, path)
	if !noted {
		head, _, err := workspaces.runner.Resolve(ctx, path, "HEAD")
		if err != nil {
			return nil, "", err
		}
		base = head
	}
	status := []string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames", "--ignore-submodules=all"}
	if includeIgnored {
		status = append(status, "--ignored=matching")
	}
	out, err := workspaces.runner.Run(ctx, git.Cmd{Dir: path, Args: status})
	if err != nil {
		return nil, "", err
	}
	var candidates []string
	for entry := range strings.SplitSeq(out, "\x00") {
		if len(entry) > 3 {
			candidates = append(candidates, entry[3:])
		}
	}
	committed, err := workspaces.runner.Run(ctx, git.Cmd{Dir: path, Args: []string{
		"diff", "--no-renames", "--name-only", "-z", "--ignore-submodules=all", "--end-of-options", base, "HEAD",
	}})
	if err != nil {
		return nil, "", err
	}
	for name := range strings.SplitSeq(committed, "\x00") {
		if name != "" {
			candidates = append(candidates, name)
		}
	}
	return candidates, base, nil
}
