package pool

import (
	"context"
	"log/slog"
	"strings"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Stage brings the commit a git directory on this disk is on into the pool, ready to be
// pushed to branch, and returns it with the subjects of the commits it brings: those no
// branch of the repository has, nor any commit gitd fetched or already published for it.
// Publishing twice does not publish the same commits twice.
//
// Pushing is done from the pool afterwards. The token is used there, where the
// configuration is gitd's, and never in the workspace, which others may write: an `http.proxy`
// or an `insteadOf` left there must not get to see where the token goes.
func (pools *Pools) Stage(ctx context.Context, repository repo.Repo, source, branch string) (string, []string, error) {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return "", nil, err
	}
	defer release()
	dir := pools.Path(repository)
	staged := "refs/gitd/staging/" + branch
	if _, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Local: true, Args: []string{
		"fetch", "--no-prune", "--update-shallow", "--end-of-options", source, "+HEAD:" + staged,
	}}); err != nil {
		return "", nil, git.About(err, repository.HTTPS())
	}
	commit, _, err := pools.runner.Resolve(ctx, dir, staged)
	if err != nil {
		return "", nil, err
	}
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Args: []string{
		"log", "--format=%s", staged, "--not", "--branches",
		"--glob=refs/gitd/commits/*", "--glob=refs/gitd/published/*",
	}})
	if err != nil {
		return "", nil, err
	}
	var subjects []string
	for line := range strings.Lines(out) {
		if subject := strings.TrimSpace(line); subject != "" {
			subjects = append(subjects, subject)
		}
	}
	return commit, subjects, nil
}

// Push sends what Stage staged for branch to a new branch of the repository, and remembers
// it as published.
//
// Only a token publishes: a deploy key is read only, and reading with nothing writes
// nothing. The push is never forced, so a branch that already exists is refused rather
// than overwritten.
func (pools *Pools) Push(
	ctx context.Context, repository repo.Repo, branch string, given []credentials.Credential,
) error {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return err
	}
	defer release()
	staged := "refs/gitd/staging/" + branch
	refspec := staged + ":refs/heads/" + branch
	access := pools.keys.For(repository, given)
	if !access.Offered || !strings.HasPrefix(access.URL, "https://") {
		failure := git.Fail(git.AuthRequired, "publishing takes a token, and none was given")
		failure.Repository = repository.HTTPS()
		return failure
	}
	_, err = pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Env: access.Env, Args: []string{
		"push", "--porcelain", "--end-of-options", access.URL, refspec,
	}})
	if err == nil {
		_, err = pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Args: []string{
			"update-ref", "refs/gitd/published/" + branch, staged,
		}})
		return err
	}
	failure, ok := git.AsFailure(err)
	if !ok {
		return err
	}
	failure.Repository = repository.HTTPS()
	failure.AccessRefused(true)
	slog.Info("not published", "repository", failure.Repository, "via", access.Via, "git_said", failure.GitSaid)
	return failure
}
