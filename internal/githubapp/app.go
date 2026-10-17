package githubapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/repo"
)

// renewBefore is how long before it expires a token is renewed rather than used: long
// enough for the longest push to finish with it.
const renewBefore = 5 * time.Minute

// permissions are what a token is limited to for each purpose.
var permissions = map[credentials.Purpose]map[string]string{
	credentials.Read:         {"contents": "read", "metadata": "read"},
	credentials.Write:        {"contents": "write", "metadata": "read"},
	credentials.PullRequests: {"pull_requests": "write", "metadata": "read"},
}

// ErrNotInstalled is a repository the App cannot reach: not installed on its owner, or not
// on that repository.
var ErrNotInstalled = fmt.Errorf("%w: the GitHub App is not installed on it", credentials.ErrUnreachable)

// App makes the installation tokens of one GitHub App, each for one repository and one
// purpose, kept in memory until shortly before they expire and never written anywhere.
type App struct {
	signer *Signer
	api    *API
	now    func() time.Time
	guard  sync.Mutex
	tokens map[tokenKey]*keptToken
}

// tokenKey is what a token is for.
type tokenKey struct {
	repository string
	purpose    credentials.Purpose
}

// keptToken is one token and until when it is good. Its guard makes those who ask at once
// wait for one token instead of asking GitHub for one each.
type keptToken struct {
	guard        sync.Mutex
	installation int64
	token        string
	expires      time.Time
}

// NewApp returns the App that signer signs for, calling GitHub's API at api.
func NewApp(signer *Signer, api *API) *App {
	return &App{signer: signer, api: api, now: time.Now, tokens: map[tokenKey]*keptToken{}}
}

// Token is a token for one repository and one purpose: the one kept, or a new one when the
// one kept is near its end.
func (app *App) Token(ctx context.Context, repository repo.Repo, purpose credentials.Purpose) (string, error) {
	owner, name, found := strings.Cut(repository.Path, "/")
	if !found || strings.Contains(name, "/") {
		return "", fmt.Errorf("%s is not a GitHub repository", repository.HTTPS())
	}
	kept := app.kept(tokenKey{repository: strings.ToLower(repository.Path), purpose: purpose})
	kept.guard.Lock()
	defer kept.guard.Unlock()
	if kept.token != "" && app.now().Add(renewBefore).Before(kept.expires) {
		return kept.token, nil
	}
	if kept.installation == 0 {
		installation, err := app.installation(ctx, owner, name)
		if err != nil {
			return "", err
		}
		kept.installation = installation
	}
	token, expires, err := app.newToken(ctx, kept.installation, name, purpose)
	if err != nil {
		kept.installation = 0
		return "", err
	}
	kept.token, kept.expires = token, expires
	return token, nil
}

func (app *App) kept(key tokenKey) *keptToken {
	app.guard.Lock()
	defer app.guard.Unlock()
	kept, found := app.tokens[key]
	if !found {
		kept = &keptToken{}
		app.tokens[key] = kept
	}
	return kept
}

// installation is the App's installation that reaches a repository.
func (app *App) installation(ctx context.Context, owner, name string) (int64, error) {
	authorization, err := app.bearer()
	if err != nil {
		return 0, err
	}
	var answer struct {
		ID int64 `json:"id"`
	}
	err = app.api.call(ctx, http.MethodGet, "/repos/"+owner+"/"+name+"/installation", authorization, nil, &answer)
	var failure *APIError
	if errors.As(err, &failure) && failure.Status == http.StatusNotFound {
		return 0, fmt.Errorf("%w: %s/%s", ErrNotInstalled, owner, name)
	}
	return answer.ID, err
}

// newToken asks GitHub for an installation token limited to one repository and purpose.
func (app *App) newToken(
	ctx context.Context, installation int64, name string, purpose credentials.Purpose,
) (string, time.Time, error) {
	authorization, err := app.bearer()
	if err != nil {
		return "", time.Time{}, err
	}
	body := map[string]any{"repositories": []string{name}, "permissions": permissions[purpose]}
	var answer struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	path := fmt.Sprintf("/app/installations/%d/access_tokens", installation)
	err = app.api.call(ctx, http.MethodPost, path, authorization, body, &answer)
	var failure *APIError
	if errors.As(err, &failure) && (failure.Status == http.StatusNotFound || failure.Status == http.StatusUnprocessableEntity) {
		return "", time.Time{}, fmt.Errorf("%w: %s (%s)", ErrNotInstalled, name, failure.Message)
	}
	if err != nil {
		return "", time.Time{}, err
	}
	if answer.Token == "" {
		return "", time.Time{}, errors.New("GitHub answered without a token")
	}
	return answer.Token, answer.ExpiresAt, nil
}

// Identity is who the App's tokens act as: its bot, e.g. `acme-agents[bot]`, and where its
// installations are managed.
func (app *App) Identity(ctx context.Context) (string, string, error) {
	authorization, err := app.bearer()
	if err != nil {
		return "", "", err
	}
	var answer struct {
		Slug    string `json:"slug"`
		HTMLURL string `json:"html_url"`
	}
	if err := app.api.call(ctx, http.MethodGet, "/app", authorization, nil, &answer); err != nil {
		return "", "", err
	}
	return answer.Slug + "[bot]", answer.HTMLURL, nil
}

func (app *App) bearer() (string, error) {
	jwt, err := app.signer.JWT(app.now())
	if err != nil {
		return "", err
	}
	return "Bearer " + jwt, nil
}
