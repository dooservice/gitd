package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// deviceGrant is the grant a device flow's token is asked for with.
const deviceGrant = "urn:ietf:params:oauth:grant-type:device_code"

// What waiting for a person to enter a code may answer besides a token.
var (
	// ErrPending is a code not entered yet: ask again after the interval.
	ErrPending = errors.New("the code has not been entered yet")
	// ErrSlowDown is asking too often: ask again after a longer interval.
	ErrSlowDown = errors.New("asked too often")
)

// OAuth signs a person in through a GitHub App by GitHub's device flow, and renews their
// tokens, with the App's client ID alone: neither takes a secret.
type OAuth struct {
	web      string
	clientID string
	client   *http.Client
}

// NewOAuth returns the device flow of the App whose client ID is clientID, on the GitHub
// whose pages are at web.
func NewOAuth(web, clientID string) *OAuth {
	return &OAuth{web: web, clientID: clientID, client: &http.Client{
		Timeout:       apiTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// DeviceCode is what a person enters, where, and how to wait for them.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// UserTokens are a person's tokens: one that works for eight hours, and one that renews it
// for six months.
type UserTokens struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
}

// AccessExpires and RefreshExpires are when the tokens stop working, counted from now.
func (tokens UserTokens) AccessExpires(now time.Time) time.Time {
	return now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
}

func (tokens UserTokens) RefreshExpires(now time.Time) time.Time {
	return now.Add(time.Duration(tokens.RefreshTokenExpiresIn) * time.Second)
}

// Start asks GitHub for a code for a person to enter.
func (oauth *OAuth) Start(ctx context.Context) (DeviceCode, error) {
	var code DeviceCode
	err := oauth.post(ctx, "/login/device/code", url.Values{"client_id": {oauth.clientID}}, &code)
	if err == nil && (code.DeviceCode == "" || code.UserCode == "") {
		err = errors.New("GitHub answered without a code")
	}
	return code, err
}

// Poll asks once whether the person entered the code: their tokens when they did,
// ErrPending or ErrSlowDown while they have not, and another error when they never will.
func (oauth *OAuth) Poll(ctx context.Context, deviceCode string) (UserTokens, error) {
	return oauth.tokens(ctx, url.Values{
		"client_id": {oauth.clientID}, "device_code": {deviceCode}, "grant_type": {deviceGrant},
	})
}

// Renew exchanges a renewal token for new tokens; the renewal token is replaced too.
func (oauth *OAuth) Renew(ctx context.Context, refreshToken string) (UserTokens, error) {
	return oauth.tokens(ctx, url.Values{
		"client_id": {oauth.clientID}, "refresh_token": {refreshToken}, "grant_type": {"refresh_token"},
	})
}

func (oauth *OAuth) tokens(ctx context.Context, form url.Values) (UserTokens, error) {
	var tokens UserTokens
	if err := oauth.post(ctx, "/login/oauth/access_token", form, &tokens); err != nil {
		return UserTokens{}, err
	}
	switch tokens.Error {
	case "":
		if tokens.AccessToken == "" {
			return UserTokens{}, errors.New("GitHub answered without a token")
		}
		return tokens, nil
	case "authorization_pending":
		return UserTokens{}, ErrPending
	case "slow_down":
		return UserTokens{}, ErrSlowDown
	default:
		return UserTokens{}, fmt.Errorf("GitHub said %s: %s", tokens.Error, tokens.ErrorDescription)
	}
}

// post sends a form to one of GitHub's pages and reads its JSON answer.
func (oauth *OAuth) post(ctx context.Context, path string, form url.Values, answer any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth.web+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := oauth.client.Do(request)
	if err != nil {
		return fmt.Errorf("calling GitHub: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return fmt.Errorf("reading GitHub's answer: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return &APIError{Status: response.StatusCode, Message: http.StatusText(response.StatusCode)}
	}
	return json.Unmarshal(data, answer)
}

// User is the login of the person a token is theirs.
func (api *API) User(ctx context.Context, accessToken string) (string, error) {
	var answer struct {
		Login string `json:"login"`
	}
	if err := api.call(ctx, http.MethodGet, "/user", "Bearer "+accessToken, nil, &answer); err != nil {
		return "", err
	}
	return answer.Login, nil
}
