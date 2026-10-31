package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// PullRequest is a pull request to open: from Head into Base.
type PullRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft"`
}

// Edit is what editing a pull request writes over: its title and body.
type Edit struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Opened is a pull request GitHub opened.
type Opened struct {
	URL    string `json:"html_url"`
	Number int64  `json:"number"`
}

// Repository is a repository on GitHub by its owner and name, and the token that reaches it.
type Repository struct {
	Owner string
	Name  string
	Token string
}

func (repository Repository) path() string {
	return "/repos/" + url.PathEscape(repository.Owner) + "/" + url.PathEscape(repository.Name)
}

func (repository Repository) authorization() string {
	return "Bearer " + repository.Token
}

// DefaultBranch is the branch a repository's pull requests merge into unless they say.
func (api *API) DefaultBranch(ctx context.Context, repository Repository) (string, error) {
	var answer struct {
		DefaultBranch string `json:"default_branch"`
	}
	err := api.call(ctx, http.MethodGet, repository.path(), repository.authorization(), nil, &answer)
	return answer.DefaultBranch, err
}

// Open opens a pull request.
func (api *API) Open(ctx context.Context, repository Repository, request PullRequest) (Opened, error) {
	var opened Opened
	err := api.call(ctx, http.MethodPost, repository.path()+"/pulls", repository.authorization(), request, &opened)
	return opened, err
}

// AskToReview asks people to review a pull request.
func (api *API) AskToReview(ctx context.Context, repository Repository, number int64, reviewers []string) error {
	path := fmt.Sprintf("%s/pulls/%d/requested_reviewers", repository.path(), number)
	return api.call(ctx, http.MethodPost, path, repository.authorization(), map[string][]string{"reviewers": reviewers}, nil)
}

// Label adds labels to a pull request.
func (api *API) Label(ctx context.Context, repository Repository, number int64, labels []string) error {
	path := fmt.Sprintf("%s/issues/%d/labels", repository.path(), number)
	return api.call(ctx, http.MethodPost, path, repository.authorization(), map[string][]string{"labels": labels}, nil)
}

// OpenFrom is the pull request open from a branch of the repository itself, and false when
// none is.
func (api *API) OpenFrom(ctx context.Context, repository Repository, branch string) (Opened, bool, error) {
	var listed []Opened
	query := url.Values{"head": {repository.Owner + ":" + branch}, "state": {"open"}}
	err := api.call(ctx, http.MethodGet, repository.path()+"/pulls?"+query.Encode(), repository.authorization(), nil, &listed)
	if err != nil || len(listed) == 0 {
		return Opened{}, false, err
	}
	return listed[0], true, nil
}

// Edit writes a pull request's title and body over what they were.
func (api *API) Edit(ctx context.Context, repository Repository, number int64, edit Edit) error {
	path := fmt.Sprintf("%s/pulls/%d", repository.path(), number)
	return api.call(ctx, http.MethodPatch, path, repository.authorization(), edit, nil)
}
