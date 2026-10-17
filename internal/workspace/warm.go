package workspace

import (
	"context"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// canStandOnWarm says why the workspace of work cannot stand on the warm layer it asks
// for, or nothing when it can or asks for none: only a new workspace, made by overlay
// from a template, stands on one.
func (workspaces *Workspaces) canStandOnWarm(work job, exists bool) error {
	if work.warm == "" {
		return nil
	}
	if err := template.ValidWarmName(work.warm); err != nil {
		return err
	}
	if exists {
		return git.Fail(git.Unsupported, "a warm layer is chosen when a workspace is made; %s exists already", work.path)
	}
	if _, isOverlay := workspaces.maker.(*strategy.OverlayMaker); !isOverlay || !workspaces.fromTemplate(work) {
		return git.Fail(git.Unsupported, "a workspace stands on a warm layer only when made by overlay, at one commit, without its whole history or its submodules")
	}
	return nil
}

// createOnWarm makes the workspace by overlay on its template, dir, with the warm layer
// above it. The layer stands on the commit it was sealed at only. Unlike a workspace on
// its template alone, it is never written instead: written, it would lack what the layer
// installed.
func (workspaces *Workspaces) createOnWarm(ctx context.Context, work job, key template.Key, dir, branch string) (Result, error) {
	warm, err := workspaces.templates.TakeWarm(ctx, work.repository, work.warm, work.path)
	if err != nil {
		_ = workspaces.templates.Let(ctx, key, work.path)
		return Result{}, err
	}
	letGo := func() {
		_ = workspaces.templates.LetWarm(ctx, warm.Dir, work.path)
		_ = workspaces.templates.Let(ctx, key, work.path)
	}
	if warm.Commit != key.Commit {
		letGo()
		return Result{}, git.Fail(git.Unsupported,
			"the warm layer %s was sealed at %s, not at %s: seal it again from a workspace at this commit", warm.Name, warm.Commit, key.Commit)
	}
	overlay := workspaces.maker.(*strategy.OverlayMaker)
	if err := overlay.MakeWarm(ctx, dir, warm.Dir, work.path); err != nil {
		letGo()
		return Result{}, err
	}
	return workspaces.finishMade(ctx, work, key.Commit, branch)
}
