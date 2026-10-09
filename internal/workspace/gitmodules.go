package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// submodule is one submodule as the superproject's `.gitmodules` names it.
type submodule struct {
	name, path, url string
}

func (submodule submodule) pending(failure *git.Failure) *gitdv1.PendingSubmodule {
	return &gitdv1.PendingSubmodule{Url: submodule.url, Path: submodule.path, Failure: failure.Proto()}
}

// declaredIn reads the submodules a superproject names, in the order it names them.
func (workspaces *Workspaces) declaredIn(ctx context.Context, dir string) ([]submodule, error) {
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

// pinned is the commit the superproject's HEAD pins each submodule's path at.
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
