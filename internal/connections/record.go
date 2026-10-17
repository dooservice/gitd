// Package connections keeps the credentials a platform gives gitd through the API: one
// file per connection under the state directory, readable by gitd's user only, its secret
// sealed when gitd has a secrets key. What is kept is applied at once: the credentials
// gitd reads and publishes with are asked for each time.
package connections

import "time"

// Kind is what a connection is.
type Kind string

const (
	// Token is a token for the repositories under a prefix, on any host.
	Token Kind = "token"
	// GitHubApp is a GitHub App: its private key makes installation tokens.
	GitHubApp Kind = "github_app"
	// GitHubLogin is a person signed in through gitd's public App: a renewal token.
	GitHubLogin Kind = "github_login"
)

// Record is one connection as it is kept. Secret is the token, the private key or the
// renewal token, as the Box keeps it; the rest is never secret. Lasting says a sign-in's
// token does not expire, as with an App that does not expire its users' tokens: Secret is
// then that token, not a renewal token.
type Record struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	URLPrefix string    `json:"url_prefix"`
	Host      string    `json:"host,omitempty"`
	User      string    `json:"user,omitempty"`
	ActsAs    string    `json:"acts_as,omitempty"`
	AppID     int64     `json:"app_id,omitempty"`
	ManageURL string    `json:"manage_url,omitempty"`
	Expires   time.Time `json:"expires,omitzero"`
	Created   time.Time `json:"created"`
	Lasting   bool      `json:"lasting,omitempty"`
	Secret    string    `json:"secret"`
}

// idPrefix starts the ID of a connection of each kind, so a person reading a list tells
// them apart.
var idPrefix = map[Kind]string{Token: "token-", GitHubApp: "app-", GitHubLogin: "login-"}
