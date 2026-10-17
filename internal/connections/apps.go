package connections

import (
	"strconv"
	"sync"

	"github.com/dooservice/gitd/internal/githubapp"
	"github.com/dooservice/gitd/internal/repo"
)

// GitHub is how gitd reaches GitHub: as which User-Agent, and on which port of each host
// that is not reached on the usual one. APIBase and WebBase, when set, are where its API
// and its pages are reached instead of their usual addresses, for every host: a proxy, or a
// test.
type GitHub struct {
	UserAgent string
	Hosts     repo.Hosts
	APIBase   string
	WebBase   string
}

// web is where a host's pages are.
func (github GitHub) web(host string) string {
	if github.WebBase != "" {
		return github.WebBase
	}
	return githubapp.WebBase(host, github.Hosts[host].HTTPSPort)
}

// api is GitHub's API for a host.
func (github GitHub) api(host string) *githubapp.API {
	base := github.APIBase
	if base == "" {
		base = githubapp.APIBase(host, github.Hosts[host].HTTPSPort)
	}
	return githubapp.NewAPI(base, github.UserAgent)
}

// apps are the GitHub Apps of the connections kept, one each, made the first time one is
// asked for and kept, with the tokens they made, until the connection goes.
type apps struct {
	guard sync.Mutex
	made  map[string]*githubapp.App
}

// app is the App of a connection: its private key read from the store each time it signs,
// so the key replaced under the connection is used from then on.
func (store *Store) app(record Record) (*githubapp.App, error) {
	store.apps.guard.Lock()
	defer store.apps.guard.Unlock()
	if made, found := store.apps.made[record.ID]; found {
		return made, nil
	}
	signer, err := githubapp.NewSigner(record.AppID, func() ([]byte, error) {
		_, key, err := store.Get(record.ID)
		return key, err
	})
	if err != nil {
		return nil, err
	}
	made := githubapp.NewApp(signer, store.github.api(record.Host))
	if store.apps.made == nil {
		store.apps.made = map[string]*githubapp.App{}
	}
	store.apps.made[record.ID] = made
	return made, nil
}

// forget drops the App of a connection removed.
func (store *Store) forget(id string) {
	store.apps.guard.Lock()
	defer store.apps.guard.Unlock()
	delete(store.apps.made, id)
}

// appLabel names an App in the logs.
func appLabel(record Record) string {
	return "GitHub App " + strconv.FormatInt(record.AppID, 10)
}
