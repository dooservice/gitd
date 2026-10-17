package connections

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dooservice/gitd/internal/githubapp"
)

// ErrUnavailable is a way of connecting this gitd does not offer, saying why.
var ErrUnavailable = errors.New("not available")

// slowerBy is how much longer to wait each time GitHub says it was asked too often.
const slowerBy = 5 * time.Second

// loginBeingFinished is a code a person is to enter, and how to wait for them.
type loginBeingFinished struct {
	host       string
	deviceCode string
	interval   time.Duration
}

// LoginToFinish is what a person enters, where, and the name to finish the sign-in by.
type LoginToFinish struct {
	UserCode        string
	VerificationURL string
	LoginID         string
	ExpiresIn       time.Duration
}

// StartLogin asks GitHub for a code a person enters to sign in through gitd's public App.
func (store *Store) StartLogin(ctx context.Context, host string) (LoginToFinish, error) {
	host, err := gitHubHost(host)
	if err != nil {
		return LoginToFinish{}, err
	}
	if store.github.LoginClientID == "" {
		return LoginToFinish{}, fmt.Errorf("%w: signing in to GitHub takes gitd's public App, and this gitd has no client ID for it", ErrUnavailable)
	}
	code, err := githubapp.NewOAuth(store.github.web(host), store.github.LoginClientID).Start(ctx)
	if err != nil {
		return LoginToFinish{}, err
	}
	expiresIn := time.Duration(code.ExpiresIn) * time.Second
	waiting := loginBeingFinished{host: host, deviceCode: code.DeviceCode, interval: time.Duration(code.Interval) * time.Second}
	id, err := store.logins.hold(waiting, store.now().Add(expiresIn))
	if err != nil {
		return LoginToFinish{}, err
	}
	return LoginToFinish{UserCode: code.UserCode, VerificationURL: code.VerificationURI, LoginID: id, ExpiresIn: expiresIn}, nil
}

// FinishLogin waits until the person has entered the code, then keeps their sign-in: the
// renewal token, sealed, and the token it renews, in memory only. It answers the
// connection and where gitd's App is installed on repositories, if it is not yet.
func (store *Store) FinishLogin(ctx context.Context, loginID string) (Record, string, error) {
	waiting, found := store.logins.take(loginID, store.now())
	if !found {
		return Record{}, "", fmt.Errorf("%w: this sign-in is not one gitd started, or it was finished or expired", ErrInvalid)
	}
	oauth := githubapp.NewOAuth(store.github.web(waiting.host), store.github.LoginClientID)
	interval := waiting.interval
	for {
		tokens, err := oauth.Poll(ctx, waiting.deviceCode)
		switch {
		case errors.Is(err, githubapp.ErrSlowDown):
			interval += slowerBy
		case errors.Is(err, githubapp.ErrPending):
		case err != nil:
			return Record{}, "", fmt.Errorf("%w: the sign-in did not finish: %v", ErrInvalid, err)
		default:
			return store.keepLogin(ctx, waiting.host, tokens)
		}
		if err := store.wait(ctx, interval); err != nil {
			return Record{}, "", err
		}
	}
}

func (store *Store) keepLogin(ctx context.Context, host string, tokens githubapp.UserTokens) (Record, string, error) {
	login, err := store.github.API(host).User(ctx, tokens.AccessToken)
	if err != nil {
		return Record{}, "", err
	}
	now := store.now()
	kept := Record{Kind: GitHubLogin, URLPrefix: "https://" + host, Host: host, ActsAs: login}
	secret := tokens.RefreshToken
	if secret == "" {
		kept.Lasting, secret = true, tokens.AccessToken
	} else {
		kept.Expires = tokens.RefreshExpires(now).UTC()
	}
	record, err := store.Add(kept, []byte(secret))
	if err != nil {
		return Record{}, "", err
	}
	if _, err := store.login(record, tokens.AccessToken, tokens.AccessExpires(now)); err != nil {
		return Record{}, "", err
	}
	install := store.github.web(host) + "/apps/" + store.github.LoginAppSlug + "/installations/new"
	return record, install, nil
}

// waitFor waits for a while, or until ctx ends.
func waitFor(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
