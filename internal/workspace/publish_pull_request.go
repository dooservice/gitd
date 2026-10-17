package workspace

import (
	"context"
	"regexp"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/pullrequests"
	"github.com/dooservice/gitd/internal/repo"
)

// accountName is what a requester's account on the host may be: a GitHub login.
var accountName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)

// PullRequestOpener opens the pull request of a published branch.
type PullRequestOpener interface {
	Open(ctx context.Context, request pullrequests.Request) (*gitdv1.OpenedPullRequest, error)
}

// validRequester refuses a requester that is not an account name, before anything is done.
func validRequester(requester string) error {
	if requester == "" || accountName.MatchString(requester) {
		return nil
	}
	return git.Fail(git.BadAddress, "%q is not an account on the host: requested_by names one, e.g. josue", requester)
}

// wantsPullRequest says whether a publish opens a pull request: always when the settings
// say so, and otherwise when the publish asks.
func wantsPullRequest(settings *gitdv1.PullRequestSettings, options *gitdv1.PullRequestOptions) bool {
	return settings.GetOpen() == gitdv1.PullRequestSettings_OPEN_ALWAYS || options.GetOpen()
}

// openPullRequest opens the pull request of a publish just pushed, telling what account
// says of it, and says in the answer what it opened, or why it could not: the branch is
// published either way.
func (workspaces *Workspaces) openPullRequest(
	ctx context.Context, request *gitdv1.PublishRequest, repository repo.Repo,
	rules publishing.Effective, account pullrequests.Account, answer *gitdv1.PublishResponse,
) {
	options := request.GetPullRequest()
	if !wantsPullRequest(rules.PullRequests, options) {
		return
	}
	opener := workspaces.publishing.PullRequests
	if opener == nil {
		answer.PullRequestFailure = git.Fail(git.Unsupported, "this gitd opens no pull requests").Proto()
		return
	}
	account.Subjects = answer.GetRepository().GetSubjects()
	for _, submodule := range answer.GetSubmodules() {
		account.Submodules = append(account.Submodules, submodule.GetPath()+" → "+request.GetBranch())
	}
	title := options.GetTitle()
	if title == "" && len(answer.GetRepository().GetSubjects()) > 0 {
		title = answer.GetRepository().GetSubjects()[0]
	}
	draft := rules.PullRequests.GetDraft()
	if options != nil && options.Draft != nil {
		draft = options.GetDraft()
	}
	var reviewers []string
	if rules.PullRequests.GetRequestReviewFromRequester() && request.GetRequestedBy() != "" {
		reviewers = []string{request.GetRequestedBy()}
	}
	opened, err := opener.Open(ctx, pullrequests.Request{
		Repository: repository, Branch: request.GetBranch(), Given: credentials.FromProto(request.GetCredentials()),
		Title: title, Body: pullrequests.Body(options.GetBody(), account), Base: options.GetBase(), Draft: draft,
		Labels: rules.PullRequests.GetLabels(), Reviewers: reviewers,
	})
	if err != nil {
		answer.PullRequestFailure = failureOf(err).Proto()
		return
	}
	answer.PullRequest = opened
}

// accountOf is what a pull request tells of a publish: the files and lines its commits
// change, read from the pool before the push marks them published, and its rules.
func (workspaces *Workspaces) accountOf(
	ctx context.Context, repository repo.Repo, request *gitdv1.PublishRequest, rules publishing.Effective,
) (pullrequests.Account, error) {
	account := pullrequests.Account{RequestedBy: request.GetRequestedBy(), Rules: rules}
	if !wantsPullRequest(rules.PullRequests, request.GetPullRequest()) {
		return account, nil
	}
	branch := request.GetBranch()
	files, err := workspaces.pools.Touched(ctx, repository, branch, pool.Paths{})
	if err != nil {
		return account, err
	}
	added, removed, err := workspaces.pools.Lines(ctx, repository, branch)
	if err != nil {
		return account, err
	}
	account.Files, account.Added, account.Removed = len(files), added, removed
	return account, nil
}

// failureOf is an error as the contract carries it.
func failureOf(err error) *git.Failure {
	if failure, ok := git.AsFailure(err); ok {
		return failure
	}
	return git.Fail(git.Failed, "%v", err)
}
