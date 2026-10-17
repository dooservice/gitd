package workspace

import (
	"context"
	"fmt"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/repo"
)

// shownPaths is at most how many paths a refusal names.
const shownPaths = 5

// Settings say what publishing a repository must follow.
type Settings interface {
	For(repository repo.Repo) publishing.Effective
}

// rulesFor is what publishing a repository must follow; nothing without settings.
func (workspaces *Workspaces) rulesFor(repository repo.Repo) publishing.Effective {
	if workspaces.publishing.Settings == nil {
		return publishing.Effective{}
	}
	return workspaces.publishing.Settings.For(repository)
}

// checkRules refuses the commits staged for branch in a repository's pool when they change
// a protected path, delete a kept one, or change more files than allowed.
func (workspaces *Workspaces) checkRules(
	ctx context.Context, repository repo.Repo, branch string, rules publishing.Effective,
) error {
	if !rules.HasRules() {
		return nil
	}
	which := "the publishing settings of " + strings.Join(rules.Applied, " and ")
	if len(rules.Protected) > 0 {
		touched, err := workspaces.pools.Touched(ctx, repository, branch, pool.Paths{Among: rules.Protected})
		if err != nil {
			return err
		}
		if len(touched) > 0 {
			return refused(repository, "%s changes %s, which %s protect", branch, listed(touched), which)
		}
	}
	if len(rules.Kept) > 0 {
		deleted, err := workspaces.pools.Touched(ctx, repository, branch, pool.Paths{Among: rules.Kept, Deleted: true})
		if err != nil {
			return err
		}
		if len(deleted) > 0 {
			return refused(repository, "%s deletes or renames away %s, which %s keep", branch, listed(deleted), which)
		}
	}
	if rules.MaxFiles > 0 {
		changed, err := workspaces.pools.Touched(ctx, repository, branch, pool.Paths{})
		if err != nil {
			return err
		}
		if len(changed) > int(rules.MaxFiles) {
			return refused(repository, "%s changes %d files, and %s allow at most %d", branch, len(changed), which, rules.MaxFiles)
		}
	}
	return nil
}

func refused(repository repo.Repo, format string, values ...any) error {
	failure := git.Fail(git.PolicyRefused, format, values...)
	failure.Repository = repository.HTTPS()
	return failure
}

// listed names the first paths, and how many more there are.
func listed(paths []string) string {
	if len(paths) <= shownPaths {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:shownPaths], ", "), len(paths)-shownPaths)
}
