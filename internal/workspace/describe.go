package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// Describe says what a workspace is on disk, without the network: its repository, its
// commit and branch, whether something was changed on disk, and its submodules.
func (workspaces *Workspaces) Describe(
	ctx context.Context, request *gitdv1.DescribeRequest,
) (*gitdv1.DescribeResponse, error) {
	path, err := workspaces.allowed(request.GetPath())
	if err != nil {
		return nil, err
	}
	config := filepath.Join(path, ".git", "config")
	if _, err := os.Stat(config); err != nil {
		return nil, git.Fail(git.NotFound, "%s is not a git repository", path)
	}
	if err := workspaces.guard(ctx, path); err != nil {
		return nil, err
	}
	described := &gitdv1.DescribeResponse{}
	if described.Url, err = workspaces.runner.Config(ctx, config, "gitd.url"); err == nil && described.Url == "" {
		described.Url, err = workspaces.runner.Config(ctx, config, "remote.origin.url")
	}
	if err != nil {
		return nil, err
	}
	described.Url = git.Redacted(described.Url)
	if described.Commit, _, err = workspaces.runner.Resolve(ctx, path, "HEAD"); err != nil {
		return nil, err
	}
	described.Branch, _ = workspaces.following(ctx, path)
	described.Strategy, described.Layers = workspaces.made(ctx, path)
	described.LfsPatterns = workspaces.LFS(path)
	status, err := workspaces.runner.Run(ctx, git.Cmd{Dir: path, Args: []string{
		"status", "--porcelain=v1", "--untracked-files=no", "--ignore-submodules=dirty",
	}})
	if err != nil {
		return nil, err
	}
	described.Modified = strings.TrimSpace(status) != ""
	described.Submodules, err = workspaces.states(ctx, path)
	return described, err
}

// Remove deletes a workspace gitd made. Its pool stays.
func (workspaces *Workspaces) Remove(ctx context.Context, request *gitdv1.RemoveRequest) error {
	path, err := workspaces.allowed(request.GetPath())
	if err != nil {
		return err
	}
	release, err := workspaces.hold(ctx, path)
	if err != nil {
		return err
	}
	defer release()
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	held, err := workspaces.madeFor(ctx, path)
	if err != nil {
		return err
	}
	if held == nil {
		return git.Fail(git.PathRefused, "%s is not a workspace gitd made", path)
	}
	if err := poolStillMounted(path); err != nil {
		return err
	}
	if workspaces.madeByMaker(path) {
		if err := workspaces.unmake(ctx, path); err != nil {
			return err
		}
	} else if err := os.RemoveAll(path); err != nil {
		return err
	}
	if err := workspaces.pools.Disown(ctx, *held, path); err != nil {
		return err
	}
	if err := workspaces.pools.ForgetBase(ctx, *held, path); err != nil {
		return err
	}
	if err := workspaces.pools.Forget(ctx, *held, path); err != nil {
		return err
	}
	workspaces.events.Tell(&gitdv1.WatchResponse{Kind: gitdv1.WatchResponse_KIND_REMOVED, Path: path})
	return nil
}
