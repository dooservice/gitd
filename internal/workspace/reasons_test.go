package workspace

import "github.com/dooservice/gitd/internal/git"

func hasReason(err error, reason git.Reason) bool {
	failure, ok := git.AsFailure(err)
	return ok && failure.Reason == reason
}
