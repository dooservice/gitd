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
// The superproject's `.git/config` points the submodule at its pool, so a console's own
// `git submodule update` reads it on this disk too.
func (workspaces *Workspaces) mount(
	ctx context.Context, work job, submodule submodule, repository repo.Repo, commit string,
) error {
	gitDir := filepath.Join(work.path, ".git", "modules", submodule.name)
	workTree := filepath.Join(work.path, submodule.path)
	poolURL := "file://" + workspaces.pools.Path(repository)
	if !exists(filepath.Join(gitDir, "HEAD")) {
		if err := workspaces.initModule(ctx, work.path, gitDir, workTree); err != nil {
			return err
		}
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

func (workspaces *Workspaces) initModule(ctx context.Context, superproject, gitDir, workTree string) error {
	if err := os.MkdirAll(filepath.Dir(gitDir), 0o755); err != nil {
		return err
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: superproject, Args: []string{"init", "--quiet", "--separate-git-dir", gitDir, workTree},
	}); err != nil {
		return err
	}
	toGitDir, err := filepath.Rel(workTree, gitDir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(workTree, ".git"), []byte("gitdir: "+toGitDir+"\n"), 0o644); err != nil {
		return err
	}
	toWorkTree, err := filepath.Rel(gitDir, workTree)
	if err != nil {
		return err
	}
	return workspaces.runner.SetConfig(ctx, workTree, "core.worktree", toWorkTree)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
