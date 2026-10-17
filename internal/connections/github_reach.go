package connections

import (
	"github.com/dooservice/gitd/internal/githubapp"
	"github.com/dooservice/gitd/internal/repo"
)

// GitHub is how gitd reaches GitHub: as which User-Agent, and on which port of each host
// that is not reached on the usual one. APIBase and WebBase, when set, are where its API
// and its pages are reached instead of their usual addresses, for every host: a proxy, or a
// test. LoginClientID and LoginAppSlug are gitd's public App, which people sign in
// through; without a client ID, signing in is not offered.
type GitHub struct {
	UserAgent     string
	Hosts         repo.Hosts
	APIBase       string
	WebBase       string
	LoginClientID string
	LoginAppSlug  string
}

// web is where a host's pages are.
func (github GitHub) web(host string) string {
	if github.WebBase != "" {
		return github.WebBase
	}
	return githubapp.WebBase(host, github.Hosts[host].HTTPSPort)
}

// API is GitHub's API for a host.
func (github GitHub) API(host string) *githubapp.API {
	base := github.APIBase
	if base == "" {
		base = githubapp.APIBase(host, github.Hosts[host].HTTPSPort)
	}
	return githubapp.NewAPI(base, github.UserAgent)
}
