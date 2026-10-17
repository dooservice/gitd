package connections

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/dooservice/gitd/internal/githubapp"
)

// manifestLifetime is how long GitHub gives to finish creating an App from a manifest.
const manifestLifetime = time.Hour

// What GitHub accepts as an App's name, as an account's login, and as the code it sends back.
var (
	appName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,33}$`)
	accountLogin = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	manifestCode = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)
)

// appBeingCreated is a manifest sent to GitHub, waiting for its code.
type appBeingCreated struct {
	host string
}

// AppToCreate is what a browser posts to GitHub to create an App: where, the manifest, and
// the state that ties GitHub's answer to it.
type AppToCreate struct {
	FormAction string
	Manifest   string
	State      string
}

// BeginApp starts creating the organisation's own App named name — or the person's, when
// organization is empty — which GitHub sends back to redirectURL with a code and the state.
func (store *Store) BeginApp(host, organization, name, redirectURL string) (AppToCreate, error) {
	host, err := gitHubHost(host)
	if err != nil {
		return AppToCreate{}, err
	}
	if organization != "" && !accountLogin.MatchString(organization) {
		return AppToCreate{}, fmt.Errorf("%w: %q is not a GitHub organisation", ErrInvalid, organization)
	}
	if !appName.MatchString(name) {
		return AppToCreate{}, fmt.Errorf("%w: an App's name has 1 to 34 letters, digits, spaces, '.', '_' or '-'", ErrInvalid)
	}
	if err := validRedirect(redirectURL); err != nil {
		return AppToCreate{}, err
	}
	manifest, err := githubapp.Manifest(name, redirectURL)
	if err != nil {
		return AppToCreate{}, err
	}
	state, err := store.creating.hold(appBeingCreated{host: host}, store.now().Add(manifestLifetime))
	if err != nil {
		return AppToCreate{}, err
	}
	return AppToCreate{FormAction: githubapp.FormAction(store.github.web(host), organization, state), Manifest: manifest, State: state}, nil
}

// CompleteApp finishes creating an App: the state must be one BeginApp gave and not yet
// used, and the code GitHub sent back is exchanged for the App's ID and private key, which
// are kept. It answers the connection and where the App is installed.
func (store *Store) CompleteApp(ctx context.Context, code, state string) (Record, string, error) {
	being, found := store.creating.take(state, store.now())
	if !found {
		return Record{}, "", fmt.Errorf("%w: this state is not one gitd gave, or it was used or expired", ErrInvalid)
	}
	if !manifestCode.MatchString(code) {
		return Record{}, "", fmt.Errorf("%w: this is not a code GitHub sends", ErrInvalid)
	}
	created, err := store.github.api(being.host).Convert(ctx, code)
	if err != nil {
		return Record{}, "", fmt.Errorf("%w: GitHub did not exchange the code: %v", ErrInvalid, err)
	}
	record, err := store.Add(Record{
		Kind: GitHubApp, URLPrefix: "https://" + being.host + "/" + created.Owner.Login, Host: being.host,
		AppID: created.ID, ActsAs: created.Slug + "[bot]", ManageURL: created.HTMLURL,
	}, []byte(created.PEM))
	if err != nil {
		return Record{}, "", err
	}
	return record, created.HTMLURL + "/installations/new", nil
}

// validRedirect accepts an address GitHub may send a person back to: on HTTPS, or on this
// machine, as a command line waiting for the code is.
func validRedirect(address string) error {
	parsed, err := url.Parse(address)
	if err == nil && parsed.Host != "" {
		local := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1"
		if parsed.Scheme == "https" || parsed.Scheme == "http" && local {
			return nil
		}
	}
	return fmt.Errorf("%w: GitHub sends the person back to an https address, or to localhost; not %q", ErrInvalid, address)
}
