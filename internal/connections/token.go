package connections

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/dooservice/gitd/internal/credentials"
)

// Errors a connection is refused with.
var (
	ErrInvalid  = errors.New("invalid connection")
	ErrFromFile = errors.New("this connection is the configuration file's: change it there")
)

// maxSecret is the most a token may hold; no host issues longer ones.
const maxSecret = 8 << 10

// NewToken is a token for the repositories under prefix, checked: a prefix that names its
// host, so the token never goes to another, and a token and user of one line each.
func NewToken(prefix, token, user string) (Record, error) {
	canonical := credentials.CanonicalPrefix(prefix)
	if !(credentials.Credential{Prefix: canonical}).NamesHost() {
		return Record{}, fmt.Errorf("%w: the prefix %q must name a host, e.g. https://gitlab.example.com/acme", ErrInvalid, prefix)
	}
	if token == "" || len(token) > maxSecret || !printable(token) {
		return Record{}, fmt.Errorf("%w: a token is one line of printable characters", ErrInvalid)
	}
	if !printable(user) || strings.Contains(user, ":") {
		return Record{}, fmt.Errorf("%w: the user %q is not a user name", ErrInvalid, user)
	}
	host, _, _ := strings.Cut(strings.TrimPrefix(canonical, "https://"), "/")
	return Record{Kind: Token, URLPrefix: canonical, Host: host, User: user}, nil
}

// printable says whether text has no control characters or spaces.
func printable(text string) bool {
	return !strings.ContainsFunc(text, func(character rune) bool {
		return unicode.IsControl(character) || unicode.IsSpace(character)
	})
}
