package pool

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Summary is what a branch brings: the subjects of its commits, newest first, how many files
// they change and how many lines they add and remove.
type Summary struct {
	Subjects []string
	Files    int
	Added    int
	Removed  int
}

// Summarize is what the commits staged for branch bring beyond the repository's branches,
// those gitd published to that branch before included: what a pull request from it is
// about, the same before the push and after. Files and lines are counted commit by commit as
// Touched reads them: a merge counts only what it brings itself, a binary file no lines.
func (pools *Pools) Summarize(ctx context.Context, repository repo.Repo, branch string) (Summary, error) {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return Summary{}, err
	}
	defer release()
	dir := pools.Path(repository)
	var summary Summary
	if summary.Subjects, err = pools.subjects(ctx, dir, onBranch(branch)); err != nil {
		return Summary{}, git.About(err, repository.HTTPS())
	}
	args := append([]string{"log", "--no-renames", "--diff-merges=remerge", "--format=", "--numstat"}, onBranch(branch)...)
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Args: args})
	if err != nil {
		return Summary{}, git.About(err, repository.HTTPS())
	}
	var files []string
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimRight(line, "\n"), "\t", 3)
		if len(fields) < 3 {
			continue
		}
		if !slices.Contains(files, fields[2]) {
			files = append(files, fields[2])
		}
		if count, err := strconv.Atoi(fields[0]); err == nil {
			summary.Added += count
		}
		if count, err := strconv.Atoi(fields[1]); err == nil {
			summary.Removed += count
		}
	}
	summary.Files = len(files)
	return summary, nil
}

// subjects are the subjects of the commits git's revisions name, newest first.
func (pools *Pools) subjects(ctx context.Context, dir string, revisions []string) ([]string, error) {
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Args: append([]string{"log", "--format=%s"}, revisions...)})
	if err != nil {
		return nil, err
	}
	var subjects []string
	for line := range strings.Lines(out) {
		if subject := strings.TrimSpace(line); subject != "" {
			subjects = append(subjects, subject)
		}
	}
	return subjects, nil
}
