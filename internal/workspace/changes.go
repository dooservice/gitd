package workspace

import (
	"context"
	"os"
	"slices"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
)

// changeSet is what was changed in a workspace: against which commit, read how, and each
// file changed, as compared with that commit.
type changeSet struct {
	path       string
	repository repo.Repo
	base       string
	fromLayer  bool
	files      []changed
}

// Changes says what was changed in a workspace since gitd last put it on a commit,
// committed or not, with the lines of each file.
//
// On an overlay workspace the files the agent touched are those of its own layer, and
// those that differ between its template and that commit: only they are compared, and no
// git runs in the workspace. Otherwise git lists them in the workspace, after the same
// checks of its .git as every other call that runs git there.
func (workspaces *Workspaces) Changes(ctx context.Context, request *gitdv1.ChangesRequest) (*gitdv1.ChangesResponse, error) {
	set, err := workspaces.changeSet(ctx, request.GetPath(), request.GetIncludeIgnored())
	if err != nil {
		return nil, err
	}
	answer := &gitdv1.ChangesResponse{Base: set.base, FromLayer: set.fromLayer}
	counted, err := workspaces.countLines(ctx, set)
	if err != nil {
		return nil, err
	}
	for _, file := range set.files {
		lines := counted[file.path]
		answer.Files = append(answer.Files, &gitdv1.ChangedFile{
			Path: file.path, Kind: file.kind, AddedLines: lines.added, RemovedLines: lines.removed, Binary: lines.binary,
		})
	}
	return answer, nil
}

// changeSet reads what was changed in the workspace at path.
func (workspaces *Workspaces) changeSet(ctx context.Context, requested string, includeIgnored bool) (changeSet, error) {
	path, err := workspaces.allowed(requested)
	if err != nil {
		return changeSet{}, err
	}
	release, err := workspaces.hold(ctx, path)
	if err != nil {
		return changeSet{}, err
	}
	defer release()
	held, err := workspaces.madeFor(ctx, path)
	if err != nil {
		return changeSet{}, err
	}
	if held == nil {
		return changeSet{}, git.Fail(git.PathRefused, "%s is not a workspace gitd made", path)
	}
	set := changeSet{path: path, repository: *held}
	root, err := os.OpenRoot(path)
	if err != nil {
		return changeSet{}, err
	}
	defer root.Close()

	var candidates []string
	if lower, upper, ok := workspaces.layers(path); ok {
		set.fromLayer = true
		candidates, set.base, err = workspaces.layerCandidates(ctx, set.repository, path, lower, upper, root)
	} else {
		candidates, set.base, err = workspaces.gitCandidates(ctx, set.repository, path, includeIgnored)
	}
	if err != nil {
		return changeSet{}, err
	}
	set.files, err = workspaces.compare(ctx, set.repository, set.base, slices.Compact(sortedCopy(candidates)), root)
	if err != nil {
		return changeSet{}, err
	}
	if set.fromLayer && !includeIgnored {
		lower, _, _ := workspaces.layers(path)
		set.files, err = workspaces.withoutIgnored(ctx, lower, set.files)
	}
	return set, err
}

// layers are an overlay workspace's template and own layer, as gitd's own record of its
// layers says, never as the workspace's configuration does.
func (workspaces *Workspaces) layers(path string) (lower, upper string, ok bool) {
	overlay, isOverlay := workspaces.maker.(*strategy.OverlayMaker)
	if !isOverlay {
		return "", "", false
	}
	lower, upper, _, ok = overlay.LayersOf(path)
	return lower, upper, ok
}

func sortedCopy(paths []string) []string {
	sorted := slices.Clone(paths)
	slices.Sort(sorted)
	return sorted
}
