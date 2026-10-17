package cli

import (
	"fmt"
	"io"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// workspaceLine says where a workspace ended up, and the submodules left behind.
func workspaceLine(out io.Writer, path string, result *gitdv1.WorkspaceResult) {
	on := "detached"
	if result.GetBranch() != "" {
		on = "on " + result.GetBranch()
	}
	fmt.Fprintf(out, "%s at %s, %s\n", path, short(result.GetCommit()), on)
	pending(out, result.GetPendingSubmodules())
}

func states(out io.Writer, submodules []*gitdv1.SubmoduleState) {
	for _, submodule := range submodules {
		state := "not checked out"
		if submodule.GetCheckedOut() {
			state = "checked out"
		}
		fmt.Fprintf(out, "  %s  %s  %s (%s)\n", short(submodule.GetCommit()), submodule.GetPath(), submodule.GetUrl(), state)
	}
}

func pending(out io.Writer, submodules []*gitdv1.PendingSubmodule) {
	for _, submodule := range submodules {
		fmt.Fprintf(out, "  pending %s (%s): %s\n", submodule.GetPath(), submodule.GetUrl(), submodule.GetFailure().GetGitSaid())
	}
}

// short is a commit as people read it.
func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// size is a number of bytes as people read it.
func size(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value, unit := float64(bytes), 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
