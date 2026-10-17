package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// Seal makes an overlay workspace's own layer a warm layer of its repository: what
// installing its dependencies wrote, for later workspaces at its template's commit to
// stand on. The layer is moved, not copied, without the workspace's .git; the workspace is
// then gone, as after Remove.
func (workspaces *Workspaces) Seal(ctx context.Context, request *gitdv1.SealRequest) (*gitdv1.WarmLayer, error) {
	if err := template.ValidWarmName(request.GetName()); err != nil {
		return nil, err
	}
	path, err := workspaces.allowed(request.GetPath())
	if err != nil {
		return nil, err
	}
	release, err := workspaces.hold(ctx, path)
	if err != nil {
		return nil, err
	}
	defer release()
	held, err := workspaces.madeFor(ctx, path)
	if err != nil {
		return nil, err
	}
	if held == nil {
		return nil, git.Fail(git.PathRefused, "%s is not a workspace gitd made", path)
	}
	record, upper, ok := workspaces.layers(path)
	if !ok {
		return nil, git.Fail(git.Unsupported, "only a workspace made by overlay can be sealed; %s was written", path)
	}
	if record.Warm != "" {
		return nil, git.Fail(git.Unsupported, "%s stands on a warm layer already; seal one made on its template", path)
	}
	key, ok := workspaces.templates.KeyOf(record.Template)
	if !ok {
		return nil, fmt.Errorf("%s is not a template", record.Template)
	}
	changed, err := workspaces.sealedChanges(ctx, path, record, upper)
	if err != nil {
		return nil, err
	}
	sealed := template.Warm{
		Repository: *held, Name: request.GetName(), Commit: key.Commit,
		Changed: changed, Sealed: time.Now().UTC(), From: path,
	}
	if err := workspaces.moveLayer(ctx, path, sealed); err != nil {
		return nil, err
	}
	if err := workspaces.forgetWorkspace(ctx, *held, record); err != nil {
		return nil, err
	}
	return warmProto(sealed), nil
}

// sealedChanges are the paths a workspace's own layer changes that git does not ignore,
// read while it is still mounted, as Changes reads them.
func (workspaces *Workspaces) sealedChanges(ctx context.Context, path string, record strategy.Record, upper string) ([]string, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	candidates, err := layerPaths(record.Lowers(), upper, root)
	if err != nil {
		return nil, err
	}
	candidates = slices.Compact(sortedCopy(candidates))
	ignored, err := workspaces.ignoredAmong(ctx, record.Template, candidates)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(candidates, func(candidate string) bool { return ignored[candidate] }), nil
}

// moveLayer lifts the workspace's own layer off it and seals it; when sealing fails, the
// workspace is put back as it was.
func (workspaces *Workspaces) moveLayer(ctx context.Context, path string, sealed template.Warm) error {
	overlay := workspaces.maker.(*strategy.OverlayMaker)
	lifted, err := overlay.Lift(path)
	if errors.Is(err, strategy.ErrHeld) {
		return git.Fail(git.Busy, "%s is still mounted elsewhere, as a sandbox binds it; stop what uses it, then seal: "+
			"what it wrote after would land in a layer other workspaces stand on", path)
	}
	if err != nil {
		return err
	}
	if err := workspaces.templates.SealWarm(ctx, sealed, lifted.Upper); err != nil {
		return errors.Join(err, lifted.Restore())
	}
	lifted.Drop()
	return nil
}

// forgetWorkspace takes a sealed workspace off everything that noted it, as Remove does.
func (workspaces *Workspaces) forgetWorkspace(ctx context.Context, repository repo.Repo, record strategy.Record) error {
	path := record.Workspace
	if err := workspaces.letLayers(ctx, record); err != nil {
		return err
	}
	if err := workspaces.pools.Disown(ctx, repository, path); err != nil {
		return err
	}
	if err := workspaces.pools.ForgetBase(ctx, repository, path); err != nil {
		return err
	}
	if err := workspaces.pools.Forget(ctx, repository, path); err != nil {
		return err
	}
	workspaces.events.Tell(&gitdv1.WatchResponse{Kind: gitdv1.WatchResponse_KIND_REMOVED, Path: path})
	return nil
}

// ListWarm are the warm layers of the repository at url.
func (workspaces *Workspaces) ListWarm(request *gitdv1.ListWarmRequest) (*gitdv1.ListWarmResponse, error) {
	repository, ok := repo.Of(request.GetUrl())
	if !ok {
		return nil, git.Unnamed(request.GetUrl())
	}
	answer := &gitdv1.ListWarmResponse{}
	if workspaces.templates == nil {
		return answer, nil
	}
	for _, warm := range workspaces.templates.Warms(repository) {
		answer.Warm = append(answer.Warm, warmProto(warm))
	}
	return answer, nil
}

func warmProto(warm template.Warm) *gitdv1.WarmLayer {
	return &gitdv1.WarmLayer{
		Url: warm.Repository.HTTPS(), Name: warm.Name, Commit: warm.Commit,
		SealedAt: warm.Sealed.Unix(), SealedFrom: warm.From,
		ChangedFiles: int32(len(warm.Changed)), Users: int32(warm.Users),
	}
}
