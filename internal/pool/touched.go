package pool

import (
	"context"
	"slices"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// unpublished are git's arguments for the commits staged for branch that would be pushed:
// those no branch of the repository has, nor any commit gitd fetched or already published.
func unpublished(branch string) []string {
	return []string{"refs/gitd/staging/" + branch, "--not", "--branches",
		"--glob=refs/gitd/commits/*", "--glob=refs/gitd/published/*"}
}

// Paths is which paths Touched lists: those among Among, git's glob pathspecs (every path
// when empty), but none among Except, and only those deleted when Deleted.
type Paths struct {
	Among   []string
	Except  []string
	Deleted bool
}

// Touched lists the paths the commits staged for branch change, as which says.
//
// Each commit is read on its own, so a change undone by a later commit is still listed:
// that history would be pushed too. A rename is a deletion and an addition. A merge lists
// only what it brings itself, beyond merging its parents as git would: merging a branch
// people changed lists nothing, and a change hidden inside a merge is listed.
func (pools *Pools) Touched(ctx context.Context, repository repo.Repo, branch string, which Paths) ([]string, error) {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return nil, err
	}
	defer release()
	args := []string{"log", "--no-renames", "--diff-merges=remerge", "--format=", "--name-only"}
	if which.Deleted {
		args = append(args, "--diff-filter=D")
	}
	args = append(args, unpublished(branch)...)
	args = append(args, "--")
	for _, pathspec := range which.Among {
		args = append(args, ":(glob)"+pathspec)
	}
	if len(which.Except) > 0 && len(which.Among) == 0 {
		args = append(args, ":(glob)**")
	}
	for _, pathspec := range which.Except {
		args = append(args, ":(exclude,glob)"+pathspec)
	}
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Args: args})
	if err != nil {
		return nil, git.About(err, repository.HTTPS())
	}
	var touched []string
	for line := range strings.Lines(out) {
		if path := strings.TrimSpace(line); path != "" && !slices.Contains(touched, path) {
			touched = append(touched, path)
		}
	}
	slices.Sort(touched)
	return touched, nil
}
