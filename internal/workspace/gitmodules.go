package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// submodule is one submodule as the superproject's `.gitmodules` names it.
type submodule struct {
	name, path, url string
}

func (submodule submodule) pending(failure *git.Failure) *gitdv1.PendingSubmodule {
	return &gitdv1.PendingSubmodule{Url: git.Redacted(submodule.url), Path: submodule.path, Failure: failure.Proto()}
}

// valid is whether the submodule's name and path stay inside the workspace, as git's own
// check_submodule_name and verify_path require. The name becomes `.git/modules/<name>` and
// the path a directory of the workspace, and both come from the repository.
func (submodule submodule) valid() bool {
	return submodule.path != "" && local(submodule.name) && local(submodule.path)
}

func local(path string) bool {
	if !filepath.IsLocal(path) || strings.Contains(path, `\`) {
		return false
	}
	return !slices.ContainsFunc(strings.Split(path, "/"), func(part string) bool {
		return part == ".." || strings.EqualFold(part, ".git")
	})
}

// declaredIn reads the submodules a superproject names, in the order it names them: those
// that may be checked out, and those whose name or path would leave the workspace.
func (workspaces *Workspaces) declaredIn(ctx context.Context, dir string) (declared, refused []submodule, err error) {
	all, err := workspaces.readGitmodules(ctx, dir)
	for _, submodule := range all {
		if submodule.valid() {
			declared = append(declared, submodule)
		} else {
			refused = append(refused, submodule)
		}
	}
	return declared, refused, err
}

// readGitmodules reads every submodule a superproject's `.gitmodules` names, as written.
func (workspaces *Workspaces) readGitmodules(ctx context.Context, dir string) ([]submodule, error) {
	file := filepath.Join(dir, ".gitmodules")
	if _, err := os.Stat(file); err != nil {
		return nil, nil
	}
	out, err := workspaces.runner.Run(ctx, git.Cmd{Dir: dir, Args: []string{
		"config", "--file", file, "--null", "--get-regexp", `^submodule\..*\.(path|url)$`,
	}})
	if failure, ok := git.AsFailure(err); ok && failure.ExitCode == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var declared []submodule
	position := map[string]int{}
	for _, entry := range strings.Split(out, "\x00") {
		key, value, _ := strings.Cut(entry, "\n")
		name, field, ok := cutLast(strings.TrimPrefix(key, "submodule."), ".")
		if !ok {
			continue
		}
		if _, seen := position[name]; !seen {
			position[name] = len(declared)
			declared = append(declared, submodule{name: name})
		}
		if field == "path" {
			declared[position[name]].path = value
		} else {
			declared[position[name]].url = value
		}
	}
	return declared, nil
}

// pinned is the commit the superproject's HEAD pins each submodule's path at. declared
// holds only valid submodules, so every path is one inside the workspace.
func (workspaces *Workspaces) pinned(ctx context.Context, dir string, declared []submodule) (map[string]string, error) {
	pins := map[string]string{}
	var paths []string
	for _, submodule := range declared {
		if submodule.path != "" {
			paths = append(paths, submodule.path)
		}
	}
	if len(paths) == 0 {
		return pins, nil
	}
	listing := git.Cmd{Dir: dir, Args: append([]string{"ls-tree", "-z", "HEAD", "--"}, paths...)}
	out, err := workspaces.runner.Run(ctx, listing)
	if err != nil {
		return nil, err
	}
	for _, entry := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if ok && len(fields) == 3 && fields[1] == "commit" {
			pins[path] = fields[2]
		}
	}
	return pins, nil
}

func cutLast(text, separator string) (before, after string, found bool) {
	at := strings.LastIndex(text, separator)
	if at < 0 {
		return text, "", false
	}
	return text[:at], text[at+len(separator):], true
}
