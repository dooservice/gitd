package workspace

import (
	"context"
	"os"
	"path/filepath"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Adopt takes in a clone gitd did not make: it links it to its repository's pool, seeding
// the pool from the clone when there is none, and repacks it so that it stops holding
// what the pool holds. What comes back is the disk the clone gave up.
//
// Nothing is removed before `fsck` has checked that the clone reads whole through the
// pool; if it does not, the link is undone and the clone is left as it was. Its checked
// out submodules are taken in the same way, each to its own pool.
func (workspaces *Workspaces) Adopt(ctx context.Context, request *gitdv1.AdoptRequest, report Report) (int64, error) {
	path, err := workspaces.allowed(request.GetPath())
	if err != nil {
		return 0, err
	}
	release, err := workspaces.hold(ctx, path)
	if err != nil {
		return 0, err
	}
	defer release()
	// A workspace gitd made is adopted already: adopting it again releases nothing.
	if held, err := workspaces.madeFor(ctx, path); err != nil || held != nil {
		return 0, err
	}
	repository, err := workspaces.originOf(ctx, path)
	if err != nil {
		return 0, err
	}
	if err := workspaces.pools.Seed(ctx, repository, path, reporter(report, repository)); err != nil {
		return 0, git.About(err, repository.HTTPS())
	}
	released, err := workspaces.takeIn(ctx, path, filepath.Join(path, ".git"), repository, report)
	if err != nil {
		return 0, err
	}
	history := "full"
	if exists(filepath.Join(path, ".git", "shallow")) {
		history = "tips"
	}
	settings := []string{
		"gitd.url", repository.HTTPS(), "gitd.pool", workspaces.pools.Path(repository), "gitd.history", history,
	}
	if branch, on := workspaces.runner.Branch(ctx, path); on {
		settings = append(settings, "gitd.branch", branch)
	}
	if err := workspaces.runner.SetConfig(ctx, path, settings...); err != nil {
		return 0, err
	}
	used, fromSubmodules, err := workspaces.adoptSubmodules(ctx, path, repository, report)
	if err != nil {
		return 0, err
	}
	return released + fromSubmodules, workspaces.note(ctx, path, append(used, repository))
}

// takeIn links one git directory to its repository's pool and repacks it without what the
// pool holds. What comes back is the disk it gave up.
func (workspaces *Workspaces) takeIn(
	ctx context.Context, workTree, gitDir string, repository repo.Repo, report Report,
) (int64, error) {
	objects := filepath.Join(gitDir, "objects")
	before := size(objects)
	if err := workspaces.link(ctx, workTree, gitDir, repository, report); err != nil {
		return 0, err
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: workTree, Progress: reporter(report, repository), Args: []string{"repack", "-a", "-d", "-l"},
	}); err != nil {
		return 0, git.About(err, repository.HTTPS())
	}
	return max(before-size(objects), 0), nil
}

// originOf is the repository a clone's origin names.
func (workspaces *Workspaces) originOf(ctx context.Context, path string) (repo.Repo, error) {
	config := filepath.Join(path, ".git", "config")
	if _, err := os.Stat(config); err != nil {
		return repo.Repo{}, git.Fail(git.NotFound, "%s is not a git repository", path)
	}
	origin, err := workspaces.runner.Config(ctx, config, "remote.origin.url")
	if err != nil {
		return repo.Repo{}, err
	}
	repository, ok := repo.Of(origin)
	if !ok {
		return repo.Repo{}, git.Unnamed(origin)
	}
	return repository, nil
}
