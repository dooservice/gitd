package connections

import (
	"context"
	"sync"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/githubapp"
	"github.com/dooservice/gitd/internal/repo"
)

// renewBefore is how long before a person's token expires it is renewed rather than used.
const renewBefore = 5 * time.Minute

// signedIn makes the tokens of a person signed in: the one kept in memory while it lasts,
// renewed with the renewal token kept in the store, which GitHub replaces each time and the
// store keeps again; or, for a token that does not expire, that token, kept sealed. A
// person's token opens what they may, whatever the purpose.
type signedIn struct {
	store   *Store
	id      string
	oauth   *githubapp.OAuth
	guard   sync.Mutex
	token   string
	expires time.Time
}

// login is the minter of a person's sign-in, holding token until expires when given.
func (store *Store) login(record Record, token string, expires time.Time) (credentials.Minter, error) {
	return store.minter(record.ID, func() (credentials.Minter, error) {
		return &signedIn{
			store: store, id: record.ID, token: token, expires: expires,
			oauth: githubapp.NewOAuth(store.github.web(record.Host), store.github.LoginClientID),
		}, nil
	})
}

func (person *signedIn) Token(ctx context.Context, _ repo.Repo, _ credentials.Purpose) (string, error) {
	person.guard.Lock()
	defer person.guard.Unlock()
	if record, kept, err := person.store.Get(person.id); err != nil || record.Lasting {
		return string(kept), err
	}
	now := person.store.now()
	if person.token != "" && now.Add(renewBefore).Before(person.expires) {
		return person.token, nil
	}
	record, renewal, err := person.store.Get(person.id)
	if err != nil {
		return "", err
	}
	tokens, err := person.oauth.Renew(ctx, string(renewal))
	if err != nil {
		return "", err
	}
	record.Expires = tokens.RefreshExpires(now).UTC()
	if err := person.store.Replace(record, []byte(tokens.RefreshToken)); err != nil {
		return "", err
	}
	person.token, person.expires = tokens.AccessToken, tokens.AccessExpires(now)
	return person.token, nil
}

// loginLabel names a sign-in in the logs.
func loginLabel(record Record) string {
	return "GitHub sign-in of " + record.ActsAs
}
