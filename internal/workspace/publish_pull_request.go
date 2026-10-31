package workspace

import (
	"context"
	"regexp"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
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

// openPullRequest opens the pull request of a publish just pushed — or, when the publish
// updates, updates the one open from its branch — telling what gitd checked of it, and says
// in the answer what it did, or why it could not: the branch is published either way.
// submodules are those on the branch, pushed now or by an earlier publish.
func (workspaces *Workspaces) openPullRequest(
	ctx context.Context, request *gitdv1.PublishRequest, repository repo.Repo,
	rules publishing.Effective, submodules []stagedSubmodule, answer *gitdv1.PublishResponse,
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
	pullRequest, err := workspaces.pullRequestOf(ctx, request, repository, rules)
	if err != nil {
		answer.PullRequestFailure = failureOf(err).Proto()
		return
	}
	for _, submodule := range submodules {
		pullRequest.account.Submodules = append(pullRequest.account.Submodules, submodule.published.GetPath()+" → "+request.GetBranch())
	}
	opened, err := opener.Open(ctx, pullRequest.written(options.GetBody()))
	if err != nil {
		answer.PullRequestFailure = failureOf(err).Proto()
		return
	}
	answer.PullRequest = opened
}

// pullRequest is a pull request to open, and the account its body tells.
type pullRequest struct {
	request pullrequests.Request
	account pullrequests.Account
}

// written is the request with its body: the platform's text, then the account.
func (pullRequest pullRequest) written(platform string) pullrequests.Request {
	request := pullRequest.request
	request.Body = pullrequests.Body(platform, pullRequest.account)
	return request
}

// pullRequestOf is the pull request of a publish's branch in a repository, as the publish
// and the repository's settings say, with what the branch brings there: read from the pool,
// those commits gitd published to it before included.
func (workspaces *Workspaces) pullRequestOf(
	ctx context.Context, request *gitdv1.PublishRequest, repository repo.Repo, rules publishing.Effective,
) (pullRequest, error) {
	options := request.GetPullRequest()
	summary, err := workspaces.pools.Summarize(ctx, repository, request.GetBranch())
	if err != nil {
		return pullRequest{}, err
	}
	title := options.GetTitle()
	if title == "" && len(summary.Subjects) > 0 {
		title = summary.Subjects[0]
	}
	draft := rules.PullRequests.GetDraft()
	if options != nil && options.Draft != nil {
		draft = options.GetDraft()
	}
	var reviewers []string
	if rules.PullRequests.GetRequestReviewFromRequester() && request.GetRequestedBy() != "" {
		reviewers = []string{request.GetRequestedBy()}
	}
	return pullRequest{
		request: pullrequests.Request{
			Repository: repository, Branch: request.GetBranch(), Given: credentials.FromProto(request.GetCredentials()),
			Title: title, Base: options.GetBase(), Draft: draft, Labels: rules.PullRequests.GetLabels(), Reviewers: reviewers,
			Update: request.GetUpdate(),
		},
		account: pullrequests.Account{
			RequestedBy: request.GetRequestedBy(), Provenance: request.GetProvenance(), Rules: rules,
			Subjects: summary.Subjects, Files: summary.Files, Added: summary.Added, Removed: summary.Removed,
		},
	}, nil
}

// failureOf is an error as the contract carries it.
func failureOf(err error) *git.Failure {
	if failure, ok := git.AsFailure(err); ok {
		return failure
	}
	return git.Fail(git.Failed, "%v", err)
}
