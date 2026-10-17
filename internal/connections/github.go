package connections

import (
	"context"
	"fmt"
	"strings"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/githubapp"
)

// defaultGitHubHost is the host a GitHub connection is for when none is said.
const defaultGitHubHost = "github.com"

// AddApp keeps a GitHub App a platform already has, once GitHub has said the key is that
// App's: what the App acts as and where it is managed come from GitHub, not from the
// caller. prefix is what it opens; empty is every repository on the host.
func (store *Store) AddApp(ctx context.Context, host string, appID int64, privateKey []byte, prefix string) (Record, error) {
	host, err := gitHubHost(host)
	if err != nil {
		return Record{}, err
	}
	if appID <= 0 {
		return Record{}, fmt.Errorf("%w: an App's ID is a positive number", ErrInvalid)
	}
	prefix, err = gitHubPrefix(host, prefix)
	if err != nil {
		return Record{}, err
	}
	signer, err := githubapp.NewSigner(appID, func() ([]byte, error) { return privateKey, nil })
	if err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	app := githubapp.NewApp(signer, store.github.API(host))
	bot, manage, err := app.Identity(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("%w: GitHub did not take this key as App %d's: %v", ErrInvalid, appID, err)
	}
	return store.Add(Record{Kind: GitHubApp, URLPrefix: prefix, Host: host, AppID: appID, ActsAs: bot, ManageURL: manage}, privateKey)
}

// gitHubHost is a GitHub host as connections name it: lowercase, github.com when empty.
func gitHubHost(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return defaultGitHubHost, nil
	}
	if strings.ContainsAny(host, "/:@ \t") {
		return "", fmt.Errorf("%w: %q is not a host name", ErrInvalid, host)
	}
	return host, nil
}

// gitHubPrefix is what a GitHub connection opens: prefix, which must be on host, or every
// repository on the host.
func gitHubPrefix(host, prefix string) (string, error) {
	if prefix == "" {
		return "https://" + host, nil
	}
	canonical := credentials.CanonicalPrefix(prefix)
	if canonical != "https://"+host && !strings.HasPrefix(canonical, "https://"+host+"/") {
		return "", fmt.Errorf("%w: the prefix %q is not on %s", ErrInvalid, prefix, host)
	}
	return canonical, nil
}
