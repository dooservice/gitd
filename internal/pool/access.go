package pool

import (
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// accessFailed is a repository gitd could not get a credential for, such as one a GitHub
// App is not installed on: refused, saying why, before git runs.
func accessFailed(repository repo.Repo, err error) *git.Failure {
	failure := git.Fail(git.AuthRefused, "%v", err)
	failure.Repository = repository.HTTPS()
	return failure
}
