package pullrequests

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/connections"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/githubapp"
	"github.com/dooservice/gitd/internal/repo"
)

// Request is a pull request to open from a published branch, with the credentials the
// publish was given. Update says to update the one already open from the branch, if one is,
// rather than open another.
type Request struct {
	Repository repo.Repo
	Branch     string
	Given      []credentials.Credential
	Title      string
	Body       string
	Base       string
	Draft      bool
	Labels     []string
	Reviewers  []string
	Update     bool
}

// Opener opens pull requests on GitHub with the credential that reaches each repository.
type Opener struct {
	keys   *credentials.Keys
	github connections.GitHub
}

// New returns an opener reaching repositories with keys, and GitHub as github says.
func New(keys *credentials.Keys, github connections.GitHub) *Opener {
	return &Opener{keys: keys, github: github}
}

// Open opens a pull request and then asks for its reviewers and labels; a review or a
// label that cannot be asked for is logged, and the pull request stays opened. Asked to
// update, it writes the title and body over those of the pull request already open from
// the branch, when one is, and leaves its base, draft, reviewers and labels as they are. A
// host gitd does not know as GitHub's is refused as unsupported.
func (opener *Opener) Open(ctx context.Context, request Request) (*gitdv1.OpenedPullRequest, error) {
	api, target, err := opener.reach(ctx, request)
	if err != nil {
		return nil, err
	}
	repository := request.Repository
	if request.Update {
		open, found, err := api.OpenFrom(ctx, target, request.Branch)
		if err != nil {
			return nil, refusedBy(repository, err)
		}
		if found {
			if err := api.Edit(ctx, target, open.Number, githubapp.Edit{Title: request.Title, Body: request.Body}); err != nil {
				return nil, refusedBy(repository, err)
			}
			return &gitdv1.OpenedPullRequest{Url: open.URL, Number: open.Number, Updated: true}, nil
		}
	}
	base := request.Base
	if base == "" {
		if base, err = api.DefaultBranch(ctx, target); err != nil {
			return nil, refusedBy(repository, err)
		}
	}
	opened, err := api.Open(ctx, target, githubapp.PullRequest{
		Title: request.Title, Body: request.Body, Head: request.Branch, Base: base, Draft: request.Draft,
	})
	if err != nil {
		return nil, refusedBy(repository, err)
	}
	if len(request.Reviewers) > 0 {
		if err := api.AskToReview(ctx, target, opened.Number, request.Reviewers); err != nil {
			slog.Warn("a review was not asked for", "pull_request", opened.URL, "reviewers", request.Reviewers, "error", err)
		}
	}
	if len(request.Labels) > 0 {
		if err := api.Label(ctx, target, opened.Number, request.Labels); err != nil {
			slog.Warn("a pull request was not labelled", "pull_request", opened.URL, "labels", request.Labels, "error", err)
		}
	}
	return &gitdv1.OpenedPullRequest{Url: opened.URL, Number: opened.Number}, nil
}

// Edit writes request's title and body over those of the pull request number.
func (opener *Opener) Edit(ctx context.Context, request Request, number int64) error {
	api, target, err := opener.reach(ctx, request)
	if err != nil {
		return err
	}
	if err := api.Edit(ctx, target, number, githubapp.Edit{Title: request.Title, Body: request.Body}); err != nil {
		return refusedBy(request.Repository, err)
	}
	return nil
}

// reach is the API of a request's repository and the repository there, with the token
// that reaches it, or why there is none: a host gitd does not know as GitHub's is refused
// as unsupported.
func (opener *Opener) reach(ctx context.Context, request Request) (*githubapp.API, githubapp.Repository, error) {
	repository := request.Repository
	owner, name, found := strings.Cut(repository.Path, "/")
	if !found || strings.Contains(name, "/") {
		return nil, githubapp.Repository{}, failure(git.Unsupported, repository,
			"pull requests are opened on GitHub, and %s is not a GitHub repository", repository.HTTPS())
	}
	token, err := opener.keys.APIToken(ctx, repository, request.Given, credentials.PullRequests)
	if err != nil {
		return nil, githubapp.Repository{}, failure(git.AuthRefused, repository, "%v", err)
	}
	if !token.Minted && repository.Host != "github.com" {
		return nil, githubapp.Repository{}, failure(git.Unsupported, repository,
			"pull requests are opened on GitHub, and gitd does not know %s as a GitHub host", repository.Host)
	}
	return opener.github.API(repository.Host), githubapp.Repository{Owner: owner, Name: name, Token: token.Token}, nil
}

// refusedBy is what GitHub's refusal means for the caller.
func refusedBy(repository repo.Repo, err error) error {
	var answered *githubapp.APIError
	if errors.As(err, &answered) && (answered.Status == 401 || answered.Status == 403) {
		return failure(git.AuthRefused, repository, "%v", err)
	}
	return failure(git.Failed, repository, "%v", err)
}

func failure(reason git.Reason, repository repo.Repo, format string, values ...any) *git.Failure {
	failed := git.Fail(reason, format, values...)
	failed.Repository = repository.HTTPS()
	return failed
}
