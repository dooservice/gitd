package pool

import (
	"context"
	"strconv"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Lines counts the lines the commits staged for branch add and remove, commit by commit as
// Touched reads them: a merge counts only what it brings itself. A binary file counts none.
func (pools *Pools) Lines(ctx context.Context, repository repo.Repo, branch string) (int, int, error) {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return 0, 0, err
	}
	defer release()
	args := append([]string{"log", "--no-renames", "--diff-merges=remerge", "--format=", "--numstat"}, unpublished(branch)...)
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Args: args})
	if err != nil {
		return 0, 0, git.About(err, repository.HTTPS())
	}
	added, removed := 0, 0
	for line := range strings.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if count, err := strconv.Atoi(fields[0]); err == nil {
			added += count
		}
		if count, err := strconv.Atoi(fields[1]); err == nil {
			removed += count
		}
	}
	return added, removed, nil
}
