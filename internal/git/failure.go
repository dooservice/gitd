package git

import (
	"errors"
	"fmt"
	"net/url"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// Reason is why an operation failed, as the contract names it.
type Reason = gitdv1.GitFailure_Reason

const (
	AuthRequired  = gitdv1.GitFailure_REASON_AUTH_REQUIRED
	AuthRefused   = gitdv1.GitFailure_REASON_AUTH_REFUSED
	NotFound      = gitdv1.GitFailure_REASON_NOT_FOUND
	BadAddress    = gitdv1.GitFailure_REASON_BAD_ADDRESS
	PathRefused   = gitdv1.GitFailure_REASON_PATH_REFUSED
	Busy          = gitdv1.GitFailure_REASON_BUSY
	Timeout       = gitdv1.GitFailure_REASON_TIMEOUT
	Corrupt       = gitdv1.GitFailure_REASON_CORRUPT
	Failed        = gitdv1.GitFailure_REASON_GIT_FAILED
	NothingNew    = gitdv1.GitFailure_REASON_NOTHING_NEW
	BranchRefused = gitdv1.GitFailure_REASON_BRANCH_REFUSED
	ConfigRefused = gitdv1.GitFailure_REASON_CONFIG_REFUSED
	PolicyRefused = gitdv1.GitFailure_REASON_POLICY_REFUSED
	Unsupported   = gitdv1.GitFailure_REASON_UNSUPPORTED
	LimitReached  = gitdv1.GitFailure_REASON_LIMIT_REACHED
)

// Failure is an operation that did not go through: why, what git said about it, and the
// repository it was about. ExitCode is git's, or -1 when git never said.
type Failure struct {
	Reason     Reason
	GitSaid    string
	Repository string
	ExitCode   int
}

func (failure *Failure) Error() string {
	if failure.Repository == "" {
		return failure.GitSaid
	}
	return failure.Repository + ": " + failure.GitSaid
}

// Proto is the failure as it travels in an error's detail.
func (failure *Failure) Proto() *gitdv1.GitFailure {
	return &gitdv1.GitFailure{Reason: failure.Reason, GitSaid: failure.GitSaid, Repository: failure.Repository}
}

// AccessRefused is what a failure about access means once it is known whether a
// credential was offered: asked for one when one was offered means that one was refused.
func (failure *Failure) AccessRefused(offered bool) *Failure {
	if offered && failure.Reason == AuthRequired {
		failure.Reason = AuthRefused
	}
	return failure
}

// AboutAccess is whether another credential could change the answer.
func (failure *Failure) AboutAccess() bool {
	return failure.Reason == AuthRequired || failure.Reason == AuthRefused || failure.Reason == NotFound
}

// Fail builds a failure git did not report, such as a path that is refused.
func Fail(reason Reason, format string, arguments ...any) *Failure {
	return &Failure{Reason: reason, GitSaid: fmt.Sprintf(format, arguments...), ExitCode: -1}
}

// Unnamed is the failure for an address that names no repository.
func Unnamed(address string) *Failure {
	return Fail(BadAddress, "%q names no repository: write it as https://host[:port]/owner/[group/…]name, "+
		"git@host:owner/name or ssh://git@host[:port]/owner/name", Redacted(address))
}

// Redacted is an address fit to log and to answer with: a user and password written in it,
// as `https://user:token@host/…`, are left out.
func Redacted(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.User == nil || parsed.Scheme == "" {
		return address
	}
	parsed.User = nil
	return parsed.String()
}

// AsFailure returns the failure inside err, if there is one.
func AsFailure(err error) (*Failure, bool) {
	var failure *Failure
	return failure, errors.As(err, &failure)
}

// About names the repository a failure is about, when it does not say yet.
func About(err error, repository string) error {
	if failure, ok := AsFailure(err); ok && failure.Repository == "" {
		failure.Repository = repository
	}
	return err
}
