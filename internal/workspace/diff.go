package workspace

import (
	"context"
	"slices"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// How much diff one call answers: unless asked, and at most.
const (
	diffBytes    = 512 << 10
	diffBytesMax = 8 << 20
)

// Diff writes the unified diff of a workspace's changes — of the files asked for, or all
// — as git writes it, from the copies Changes compares, cut short at a size.
func (workspaces *Workspaces) Diff(ctx context.Context, request *gitdv1.DiffRequest) (*gitdv1.DiffResponse, error) {
	set, err := workspaces.changeSet(ctx, request.GetPath(), request.GetIncludeIgnored())
	if err != nil {
		return nil, err
	}
	limit := request.GetMaxBytes()
	if limit <= 0 {
		limit = diffBytes
	}
	limit = min(limit, diffBytesMax)
	files := set.files
	if asked := request.GetFiles(); len(asked) > 0 {
		files = slices.DeleteFunc(slices.Clone(files), func(file changed) bool { return !slices.Contains(asked, file.path) })
	}
	answer := &gitdv1.DiffResponse{Base: set.base}
	if len(files) == 0 {
		return answer, nil
	}
	copied, err := workspaces.copyAside(ctx, set, files)
	if err != nil {
		return nil, err
	}
	defer copied.remove()
	var written strings.Builder
	for _, file := range files {
		out, err := workspaces.runner.Run(ctx, git.Cmd{Dir: copied.directory, Differences: true, Args: []string{
			"diff", "--no-index", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-prefix", "--",
			copied.side("a", file.path), copied.side("b", file.path),
		}})
		if err != nil {
			return nil, err
		}
		piece := withGitsHeader(out, file.path)
		if int64(written.Len()+len(piece)) > limit {
			written.WriteString(piece[:max(0, int(limit)-written.Len())])
			answer.Truncated = true
			break
		}
		written.WriteString(piece)
	}
	answer.Diff = written.String()
	return answer, nil
}

// withGitsHeader writes a file's diff, made between copies aside, with the header git
// writes in a repository: `diff --git a/<path> b/<path>`, whether the file was added,
// deleted or modified.
func withGitsHeader(out, path string) string {
	first, rest, found := strings.Cut(out, "\n")
	if !found || !strings.HasPrefix(first, "diff --git ") {
		return out
	}
	return "diff --git a/" + path + " b/" + path + "\n" + rest
}
