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
