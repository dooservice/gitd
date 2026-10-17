package pool

import (
	"context"
	"slices"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Probe asks a repository whether it can be read with the credential that opens it, or
// with nothing when none was given. The same answer lists its branches, so nobody has to
// type one from memory.
func (pools *Pools) Probe(ctx context.Context, request *gitdv1.ProbeRequest) *gitdv1.ProbeResponse {
	repository, ok := repo.Of(request.GetUrl())
	if !ok {
		return denied(git.Unnamed(request.GetUrl()))
	}
	access := pools.keys.For(repository, credentials.FromProto(request.GetCredentials()))
	out, err := pools.runner.Run(ctx, git.Cmd{Dir: "/", Env: access.Env, Args: []string{
		"ls-remote", "--symref", "--end-of-options", access.URL, "HEAD", "refs/heads/*",
	}})
	if err == nil {
		answer := listed(out)
		answer.Access = gitdv1.ProbeResponse_ACCESS_PUBLIC
		if access.Offered {
			answer.Access = gitdv1.ProbeResponse_ACCESS_REACHABLE
		}
		return answer
	}
	failure, ok := git.AsFailure(err)
	if !ok {
		failure = git.Fail(git.Failed, "%v", err)
	}
	failure.Repository = repository.HTTPS()
	return denied(failure.AccessRefused(access.Offered))
}

func denied(failure *git.Failure) *gitdv1.ProbeResponse {
	return &gitdv1.ProbeResponse{Access: gitdv1.ProbeResponse_ACCESS_DENIED, Failure: failure.Proto()}
}

// listed reads `ls-remote --symref`: the branches, the commit each points at, and the one
// HEAD points at.
func listed(out string) *gitdv1.ProbeResponse {
	answer := &gitdv1.ProbeResponse{Commits: map[string]string{}}
	for line := range strings.Lines(out) {
		left, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		if target, symbolic := strings.CutPrefix(left, "ref: "); symbolic && ref == "HEAD" {
			answer.DefaultBranch = strings.TrimPrefix(target, "refs/heads/")
		} else if branch, isBranch := strings.CutPrefix(ref, "refs/heads/"); isBranch {
			answer.Branches = append(answer.Branches, branch)
			answer.Commits[branch] = left
		}
	}
	slices.Sort(answer.Branches)
	return answer
}
