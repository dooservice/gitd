package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// fromTemplate says whether a new workspace for work is made from a template: when a maker
// was picked, and for what templates hold so far — one commit, without its whole history
// or its submodules. Anything else is made by writing its files.
func (workspaces *Workspaces) fromTemplate(work job) bool {
	if workspaces.maker == nil || workspaces.templates == nil || work.full || work.submodules {
		return false
	}
	return work.strategy == "" || work.strategy == workspaces.maker.Name()
}

// createFromTemplate makes the workspace from the template of its commit, without writing
// its files, and puts it on its branch by moving refs only: the template is already at the
// commit, so nothing in the tree changes. When the maker cannot run for this path, the
// workspace is made by writing its files instead.
func (workspaces *Workspaces) createFromTemplate(ctx context.Context, work job) (Result, error) {
	commit, branch, err := workspaces.target(ctx, work)
	if err != nil {
		return Result{}, err
	}
	key := template.Key{Repository: work.repository, Commit: commit}
	dir, err := workspaces.templates.Take(ctx, key, work.path)
	if err != nil {
		return Result{}, err
	}
	if err := workspaces.maker.Make(ctx, dir, work.path); err != nil {
		_ = workspaces.templates.Let(ctx, key, work.path)
		slog.Warn("workspace written instead", "path", work.path, "strategy", workspaces.maker.Name(), "error", err)
		return workspaces.create(ctx, work)
	}
	if err := workspaces.onBranch(ctx, work.path, commit, branch); err != nil {
		_ = workspaces.unmake(ctx, work.path)
		return Result{}, err
	}
	if err := workspaces.runner.SetConfig(ctx, work.path, "gitd.strategy", string(workspaces.maker.Name())); err != nil {
		_ = workspaces.unmake(ctx, work.path)
		return Result{}, err
	}
	made, layers := workspaces.made(ctx, work.path)
	result := Result{Commit: commit, Branch: branch, Strategy: made, Layers: layers}
	return result, workspaces.note(ctx, work.path, []repo.Repo{work.repository})
}

// target is the commit a job goes to, in full, and the branch it follows, if any: the commit
// asked for, detached; or the tip of the branch asked for, or of the default branch.
func (workspaces *Workspaces) target(ctx context.Context, work job) (commit, branch string, err error) {
	revision := work.revision.GetCommit()
	if revision == "" {
		branch = work.revision.GetBranch()
		if branch == "" {
			branch = workspaces.pools.DefaultBranch(ctx, work.repository)
		}
		if err := workspaces.validBranch(ctx, branch); err != nil {
			return "", "", err
		}
		revision = "refs/heads/" + branch
	}
	commit, present, err := workspaces.pools.Has(ctx, work.repository, revision)
	if err != nil {
		return "", "", err
	}
	if !present {
		failure := git.Fail(git.NotFound, "%s is not in the repository", revision)
		failure.Repository = work.repository.HTTPS()
		return "", "", failure
	}
	return commit, branch, nil
}

func (workspaces *Workspaces) validBranch(ctx context.Context, branch string) error {
	if branch == "" {
		return git.Fail(git.NotFound, "the repository names no default branch")
	}
	// Checked as refs/heads/<branch>: an argument starting with refs/ cannot pass for an
	// option, and `check-ref-format` knows no --end-of-options.
	if strings.HasPrefix(branch, "-") {
		return git.Fail(git.BadAddress, "%q is not a branch name", branch)
	}
	if _, err := workspaces.runner.Run(ctx, git.Cmd{
		Dir: "/", Args: []string{"check-ref-format", "refs/heads/" + branch},
	}); err != nil {
		return git.Fail(git.BadAddress, "%q is not a branch name", branch)
	}
	return nil
}

// onBranch puts a workspace made from a template — detached at commit — on branch, where a
// written workspace's `checkout -B branch origin/branch` leaves it, by writing refs and
// configuration only. No branch leaves it detached.
func (workspaces *Workspaces) onBranch(ctx context.Context, path, commit, branch string) error {
	if branch == "" {
		return nil
	}
	for _, args := range [][]string{
		{"update-ref", "--end-of-options", "refs/remotes/origin/" + branch, commit},
		{"update-ref", "--end-of-options", "refs/heads/" + branch, commit},
		{"symbolic-ref", "HEAD", "refs/heads/" + branch},
	} {
		if _, err := workspaces.runner.Run(ctx, git.Cmd{Dir: path, Args: args}); err != nil {
			return fmt.Errorf("putting %s on %s: %w", path, branch, err)
		}
	}
	return workspaces.runner.SetConfig(ctx, path, "gitd.branch", branch)
}

// unmake removes a workspace a maker made, and lets go of its template.
func (workspaces *Workspaces) unmake(ctx context.Context, path string) error {
	overlay, ok := workspaces.maker.(*strategy.OverlayMaker)
	if !ok {
		return workspaces.maker.Unmake(ctx, path)
	}
	for _, record := range overlay.Records() {
		if record.Workspace != path {
			continue
		}
		if err := overlay.Unmake(ctx, path); err != nil {
			return err
		}
		if key, ok := workspaces.templates.KeyOf(record.Template); ok {
			return workspaces.templates.Let(ctx, key, path)
		}
		return nil
	}
	return nil
}

// madeByMaker says whether the workspace at path was made by the maker in use, so that it
// is unmade rather than deleted file by file.
func (workspaces *Workspaces) madeByMaker(path string) bool {
	overlay, ok := workspaces.maker.(*strategy.OverlayMaker)
	if !ok {
		return false
	}
	for _, record := range overlay.Records() {
		if record.Workspace == path {
			return true
		}
	}
	return false
}

// made is how the workspace at path was made, as it notes in gitd.strategy — copy for one
// made before gitd noted it — and its layers when by overlay.
func (workspaces *Workspaces) made(ctx context.Context, path string) (gitdv1.Strategy, *gitdv1.Layers) {
	noted, _ := workspaces.runner.Config(ctx, filepath.Join(path, ".git", "config"), "gitd.strategy")
	name := strategy.Name(strings.TrimSpace(noted))
	if name == "" {
		name = strategy.Copy
	}
	overlay, ok := workspaces.maker.(*strategy.OverlayMaker)
	if !ok || name != strategy.Overlay {
		return name.Proto(), nil
	}
	lower, upper, work, ok := overlay.LayersOf(path)
	if !ok {
		return name.Proto(), nil
	}
	return name.Proto(), &gitdv1.Layers{Lower: lower, Upper: upper, Work: work}
}
