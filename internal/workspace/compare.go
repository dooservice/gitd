package workspace

import (
	"context"
	"slices"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// Compare says how two works of one repository differ, file by file from the first to the
// second, with their lines and, when asked, their diff.
//
// Only what may differ is compared: what each workspace changed since gitd put it on its
// commit, and what differs between the two commits. No tree is walked whole, and on
// overlay workspaces no git runs in a workspace.
func (workspaces *Workspaces) Compare(ctx context.Context, request *gitdv1.CompareRequest) (*gitdv1.CompareResponse, error) {
	first, err := workspaces.openSide(ctx, request.GetFirst(), request.GetUrl(), request.GetIncludeIgnored())
	if err != nil {
		return nil, err
	}
	defer first.close()
	second, err := workspaces.openSide(ctx, request.GetSecond(), request.GetUrl(), request.GetIncludeIgnored())
	if err != nil {
		return nil, err
	}
	defer second.close()
	if first.repository != second.repository {
		return nil, git.Fail(git.BadAddress, "%s and %s are works of different repositories", first.name, second.name)
	}
	paths, err := workspaces.comparedPaths(ctx, first, second)
	if err != nil {
		return nil, err
	}
	format, err := workspaces.pools.ObjectFormat(ctx, first.repository)
	if err != nil {
		return nil, err
	}
	var differing []changed
	for _, path := range paths {
		firstObject, firstMode, inFirst, err := first.object(path, format)
		if err != nil {
			return nil, err
		}
		secondObject, secondMode, inSecond, err := second.object(path, format)
		if err != nil {
			return nil, err
		}
		switch {
		case inFirst && !inSecond:
			differing = append(differing, changed{path: path, kind: gitdv1.ChangedFile_KIND_DELETED, baseObject: firstObject})
		case !inFirst && inSecond:
			differing = append(differing, changed{path: path, kind: gitdv1.ChangedFile_KIND_ADDED})
		case inFirst && inSecond && (firstObject != secondObject || firstMode != secondMode):
			differing = append(differing, changed{path: path, kind: gitdv1.ChangedFile_KIND_MODIFIED, baseObject: firstObject})
		}
	}
	return workspaces.compared(ctx, first, second, differing, request)
}

// comparedPaths are the paths two works may differ on, as files: what each changed since
// its commit, and what differs between the commits, each directory standing for the files
// under it; what is inside a submodule is the submodule's.
func (workspaces *Workspaces) comparedPaths(ctx context.Context, first, second *side) ([]string, error) {
	candidates := append(slices.Clone(first.changed), second.changed...)
	if first.base != second.base {
		between, err := workspaces.pools.ChangedBetween(ctx, first.repository, first.base, second.base)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, between...)
	}
	var submodules []string
	for _, opened := range []*side{first, second} {
		named, err := workspaces.pools.Submodules(ctx, opened.repository, opened.base)
		if err != nil {
			return nil, err
		}
		submodules = append(submodules, named...)
	}
	var expanded []string
	for _, opened := range []*side{first, second} {
		if opened.root == nil {
			continue
		}
		paths, err := opened.expand(candidates)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, paths...)
	}
	expanded = append(expanded, candidates...)
	for _, opened := range []*side{first, second} {
		if err := workspaces.load(ctx, opened, expanded); err != nil {
			return nil, err
		}
		if opened.root == nil {
			paths, err := opened.expand(nil)
			if err != nil {
				return nil, err
			}
			expanded = append(expanded, paths...)
		}
	}
	slices.Sort(expanded)
	return slices.DeleteFunc(slices.Compact(expanded), func(path string) bool {
		path = strings.TrimSuffix(path, "/")
		return path == "" || insideAny(path, submodules) || first.isDirectory(path) || second.isDirectory(path)
	}), nil
}

// compared answers the differing files with their lines, and the diff when asked, from
// copies of both sides made aside.
func (workspaces *Workspaces) compared(
	ctx context.Context, first, second *side, differing []changed, request *gitdv1.CompareRequest,
) (*gitdv1.CompareResponse, error) {
	answer := &gitdv1.CompareResponse{}
	if len(differing) == 0 {
		return answer, nil
	}
	copied, err := workspaces.copySides(ctx, first, second, differing)
	if err != nil {
		return nil, err
	}
	defer copied.remove()
	counted, err := workspaces.numstat(ctx, copied)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(differing))
	for index, file := range differing {
		paths[index] = file.path
		lines := counted[file.path]
		answer.Files = append(answer.Files, &gitdv1.ChangedFile{
			Path: file.path, Kind: file.kind, AddedLines: lines.added, RemovedLines: lines.removed, Binary: lines.binary,
		})
	}
	if request.GetWithDiff() {
		answer.Diff, answer.Truncated, err = workspaces.diffAside(ctx, copied, paths, diffLimit(request.GetMaxBytes()))
	}
	return answer, err
}

// copySides copies each differing file aside: the first side's under a/, the second's
// under b/.
func (workspaces *Workspaces) copySides(ctx context.Context, first, second *side, differing []changed) (*aside, error) {
	directory, err := newAside()
	if err != nil {
		return nil, err
	}
	for _, file := range differing {
		for name, opened := range map[string]*side{"a": first, "b": second} {
			if err := workspaces.copySide(ctx, directory, name, opened, file.path); err != nil {
				directory.remove()
				return nil, err
			}
		}
	}
	return directory, nil
}

// copySide copies one side's file aside, if the side has it.
func (workspaces *Workspaces) copySide(ctx context.Context, copied *aside, name string, opened *side, path string) error {
	if opened.root != nil {
		return copied.fromWorkspace(opened.root, name, path)
	}
	entry, present := opened.entries[path]
	if !present {
		return nil
	}
	content, err := workspaces.pools.Blob(ctx, opened.repository, entry.Object)
	if err != nil {
		return err
	}
	return copied.write(name, path, strings.NewReader(content))
}
