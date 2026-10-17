package workspace

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// mount checks a submodule out the way `git submodule update` would leave it — its git
// directory under `.git/modules/<name>`, a `.git` file pointing there and `core.worktree`
// pointing back, both relative so the workspace can be renamed into place — but borrowing
// from its pool.
//
// It is done by hand because `submodule update --reference` refuses a shallow pool, and
// without the reference it copies every object the pool is there to hold once.
//
// The superproject's `.git/config` points the submodule at its pool, so a person's own
// `git submodule update` reads it on this disk too.
//
// Its name and path come from the repository, and whoever works in the workspace may have
// left a symbolic link where they lead: every directory and file gitd makes for it is made
// through an os.Root of the workspace, which refuses to follow a link out of it.
func (workspaces *Workspaces) mount(
	ctx context.Context, work job, submodule submodule, repository repo.Repo, commit string,
) error {
	workspace, err := os.OpenRoot(work.path)
	if err != nil {
		return err
	}
	defer workspace.Close()
	gitDirIn := filepath.Join(".git", "modules", submodule.name)
	gitDir, workTree := filepath.Join(work.path, gitDirIn), filepath.Join(work.path, submodule.path)
	poolURL := "file://" + workspaces.pools.Path(repository)
	if err := workspace.MkdirAll(submodule.path, 0o755); err != nil {
		return git.Fail(git.PathRefused, "submodule %s: %v", submodule.path, err)
	}
	if _, err := workspace.Stat(filepath.Join(gitDirIn, "HEAD")); err != nil {
		if err := workspaces.initModule(ctx, workspace, work.path, gitDirIn); err != nil {
			return err
		}
	}
	// Also when the git directory was already there: the submodule may have moved, or its
	// checkout been removed, and git run in a directory with no `.git` of its own would
	// write to the superproject instead.
	if err := workspaces.pointAt(ctx, workspace, gitDirIn, submodule.path); err != nil {
		return err
	}
	if err := workspaces.borrow(gitDir, repository); err != nil {
		return err
	}
	if err := workspaces.runner.SetConfig(ctx, workTree, "remote.origin.url", poolURL); err != nil {
		return err
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: workTree, Progress: reporter(work.report, repository),
		Args: []string{"checkout", "--force", "--progress", "--detach", commit},
	}); err != nil {
		return err
	}
	return workspaces.runner.SetConfig(ctx, work.path,
		"submodule."+submodule.name+".url", poolURL,
		"submodule."+submodule.name+".active", "true",
	)
}

// initModule makes a submodule's git directory, gitDir within the workspace, as a bare
// repository; pointAt then links it to its checkout.
func (workspaces *Workspaces) initModule(ctx context.Context, workspace *os.Root, superproject, gitDir string) error {
	if err := workspace.MkdirAll(gitDir, 0o755); err != nil {
		return git.Fail(git.PathRefused, "%s: %v", gitDir, err)
	}
	_, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: filepath.Join(superproject, gitDir), Args: []string{"init", "--quiet", "--bare"},
	})
	return err
}

// pointAt links a submodule's checkout and its git directory both ways, with relative
// paths: the checkout's `.git` file and the git directory's `core.worktree`, written as
// `submodule update` leaves them, with core.bare off.
func (workspaces *Workspaces) pointAt(ctx context.Context, workspace *os.Root, gitDir, workTree string) error {
	toGitDir, err := filepath.Rel(workTree, gitDir)
	if err != nil {
		return err
	}
	if err := workspace.WriteFile(filepath.Join(workTree, ".git"), []byte("gitdir: "+toGitDir+"\n"), 0o644); err != nil {
		return git.Fail(git.PathRefused, "submodule %s: %v", workTree, err)
	}
	toWorkTree, err := filepath.Rel(gitDir, workTree)
	if err != nil {
		return err
	}
	return workspaces.runner.SetConfig(ctx, filepath.Join(workspace.Name(), gitDir),
		"core.bare", "false", "core.logallrefupdates", "true", "core.worktree", toWorkTree)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
