package workspace

import (
	"context"
	"os"
	"path/filepath"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// states is what each submodule is on disk: the commit it is at when it is checked out,
// and the one pinned when it is not.
func (workspaces *Workspaces) states(ctx context.Context, dir string) ([]*gitdv1.SubmoduleState, error) {
	declared, err := workspaces.declaredIn(ctx, dir)
	if err != nil {
		return nil, err
	}
	pins, err := workspaces.pinned(ctx, dir, declared)
	if err != nil {
		return nil, err
	}
	states := []*gitdv1.SubmoduleState{}
	for _, submodule := range declared {
		if submodule.path == "" || submodule.url == "" {
			continue
		}
		state := &gitdv1.SubmoduleState{Path: submodule.path, Url: submodule.url, Commit: pins[submodule.path]}
		checkout := filepath.Join(dir, submodule.path)
		if _, err := os.Stat(filepath.Join(checkout, ".git")); err == nil {
			state.CheckedOut = true
			if commit, present, err := workspaces.runner.Resolve(ctx, checkout, "HEAD"); err == nil && present {
				state.Commit = commit
			}
		}
		states = append(states, state)
	}
	return states, nil
}
