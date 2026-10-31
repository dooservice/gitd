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
	return pools.StageRef(ctx, repository, source, "HEAD", branch)
}

// StageRef is Stage of the commit ref names in the git directory source rather than the
// one it is on: a branch a sandbox pushed to its sandbox repository.
func (pools *Pools) StageRef(ctx context.Context, repository repo.Repo, source, ref, branch string) (string, []string, error) {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return "", nil, err
	}
	defer release()
	dir := pools.Path(repository)
	staged := "refs/gitd/staging/" + branch
	if _, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Local: true, Args: []string{
		"fetch", "--no-prune", "--update-shallow", "--end-of-options", source, "+" + ref + ":" + staged,
	}}); err != nil {
		return "", nil, git.About(err, repository.HTTPS())
	}
	commit, _, err := pools.runner.Resolve(ctx, dir, staged)
	if err != nil {
		return "", nil, err
	}
	subjects, err := pools.subjects(ctx, dir, unpublished(branch))
	if err != nil {
		return "", nil, err
	}
	return commit, subjects, nil
}

// Holds says whether the branch gitd published holds what Stage staged for it, and that
// holds commits beyond the repository's branches: a publish bringing nothing new to a
// branch an earlier one made, whose pull requests may still be missing.
func (pools *Pools) Holds(ctx context.Context, repository repo.Repo, branch string) (bool, error) {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return false, err
	}
	defer release()
	dir := pools.Path(repository)
	published := "refs/gitd/published/" + branch
	if _, found, err := pools.runner.Resolve(ctx, dir, published); err != nil || !found {
		return false, err
	}
	if beyond, err := pools.anyCommit(ctx, dir, "refs/gitd/staging/"+branch, "--not", published); err != nil || beyond {
		return false, git.About(err, repository.HTTPS())
	}
	own, err := pools.anyCommit(ctx, dir, onBranch(branch)...)
	return own, git.About(err, repository.HTTPS())
}

// anyCommit is whether git's revisions name any commit.
func (pools *Pools) anyCommit(ctx context.Context, dir string, revisions ...string) (bool, error) {
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Args: append([]string{"log", "-1", "--format=%H"}, revisions...)})
	return strings.TrimSpace(out) != "", err
}

// Push sends what Stage staged for branch to a new branch of the repository, and remembers
// it as published. Forward lets the branch exist already and moves it, only ever forward.
//
// Only a token publishes: a deploy key is read only, and reading with nothing writes
// nothing. The push is never forced: a branch that already exists is refused rather than
// overwritten, and one moved forward keeps every commit it had.
func (pools *Pools) Push(
	ctx context.Context, repository repo.Repo, branch string, given []credentials.Credential, forward bool,
) error {
	release, err := pools.Lock(ctx, repository)
	if err != nil {
		return err
	}
	defer release()
	staged := "refs/gitd/staging/" + branch
	refspec := staged + ":refs/heads/" + branch
	access, err := pools.keys.Access(ctx, repository, given, credentials.Write)
	if err != nil {
		return accessFailed(repository, err)
	}
	if !access.Offered || !strings.HasPrefix(access.URL, "https://") {
		failure := git.Fail(git.AuthRequired, "publishing takes a token, and none was given")
		failure.Repository = repository.HTTPS()
		return failure
	}
	args := []string{"push", "--porcelain"}
	if !forward {
		// A lease on no value at all: the branch must not exist on the host.
		args = append(args, "--force-with-lease=refs/heads/"+branch+":")
	}
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: pools.Path(repository), Env: access.Env, Args: append(args,
		"--end-of-options", access.URL, refspec,
	)})
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
	if rejected(out) {
		failure.Reason, failure.GitSaid = git.BranchExists, branch+" already exists on the host: publish with update to move it forward"
		if forward {
			failure.GitSaid = branch + " on the host has commits this publish does not: it is moved only forward, never over them"
		}
	}
	failure.Repository = repository.HTTPS()
	failure.AccessRefused(true)
	slog.Info("not published", "repository", failure.Repository, "via", access.Via, "git_said", failure.GitSaid)
	return failure
}

// rejected is whether git's porcelain answer to a push says the host's branch was not
// moved because of what it holds: it exists, or it is not behind what was pushed.
func rejected(porcelain string) bool {
	for line := range strings.Lines(porcelain) {
		if strings.HasPrefix(line, "!") && strings.Contains(line, "\t[rejected]") {
			return true
		}
	}
	return false
}

// Unstage forgets what Stage staged for branch, once the publish is refused or fails: what
// it held is no one else's to see.
func (pools *Pools) Unstage(ctx context.Context, repository repo.Repo, branch string) {
	release, err := pools.Lock(context.WithoutCancel(ctx), repository)
	if err != nil {
		return
	}
	defer release()
	_, _ = pools.runner.Run(context.WithoutCancel(ctx), git.Cmd{Dir: pools.Path(repository), Args: []string{
		"update-ref", "-d", "refs/gitd/staging/" + branch,
	}})
}
