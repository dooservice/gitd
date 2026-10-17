package pool

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/progress"
)

// fetch brings what is wanted into the pool at dir. A pool that holds only tips keeps
// holding only tips, and one that holds the whole history keeps it: a commit or a branch
// is never fetched shallow into it.
//
// The remote's HEAD is asked first, with the same credential: it is which branch a
// workspace with none gets, and it is followed, so a repository whose main branch went
// from `master` to `main` hands out the new one.
//
// A pool being created holds nothing yet, and is shallow unless the whole history is
// wanted.
func (pools *Pools) fetch(
	ctx context.Context, dir string, access credentials.Access, want Want, creating bool, report progress.Report,
) error {
	head, err := pools.remoteHead(ctx, dir, access)
	if err != nil {
		return err
	}
	if head != "" {
		_, _ = pools.runner.Run(ctx, git.Cmd{Dir: dir, Args: []string{"symbolic-ref", "HEAD", "refs/heads/" + head}})
	}
	shallow := creating || exists(filepath.Join(dir, "shallow"))
	args := []string{"fetch", "--no-prune", "--progress"}
	var refspecs []string
	switch {
	case want.Full:
		if exists(filepath.Join(dir, "shallow")) {
			args = append(args, "--unshallow")
		}
		refspecs = []string{"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"}
	default:
		if shallow {
			args = append(args, "--depth", "1")
		}
		branches := want.Branches
		if len(branches) == 0 && len(want.Commits) == 0 {
			if head == "" {
				return git.Fail(git.NotFound, "the repository names no default branch")
			}
			branches = []string{head}
		}
		for _, branch := range branches {
			refspecs = append(refspecs, "+refs/heads/"+branch+":refs/heads/"+branch)
		}
		for _, commit := range want.Commits {
			refspecs = append(refspecs, commit+":refs/gitd/commits/"+commit)
		}
	}
	args = append(append(args, "--end-of-options", access.URL), refspecs...)
	_, err = pools.runner.Run(ctx, git.Cmd{Dir: dir, Env: access.Env, Progress: report, Args: args})
	return err
}

// remoteHead is the branch the remote's HEAD points at, or "" when it does not say. A
// remote that cannot be read is an error: what git said about it is the answer, where the
// fetch after it would only fail the same way.
func (pools *Pools) remoteHead(ctx context.Context, dir string, access credentials.Access) (string, error) {
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: dir, Env: access.Env, Args: []string{
		"ls-remote", "--symref", "--end-of-options", access.URL, "HEAD",
	}})
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(out) {
		target, ref, symbolic := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "ref: "), "\t")
		if branch, isBranch := strings.CutPrefix(target, "refs/heads/"); symbolic && ref == "HEAD" && isBranch {
			return branch, nil
		}
	}
	return "", nil
}
