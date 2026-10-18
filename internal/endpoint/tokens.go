// Package endpoint is gitd's git endpoint: sandboxes that cannot mount gitd's directories
// clone and push over smart HTTP, with tokens of gitd's own that open one repository each,
// never the git host's credential.
//
// A token opens one repository for reading and one sandbox repository for writing. The
// sandbox repository borrows the pool's objects and takes the sandbox's pushes; publishing
// takes a branch from it to the git host through the same checks as publishing a
// workspace. Tokens are kept hashed, in memory only, and expire.
package endpoint

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

const (
	// DefaultLife is how long a token lives when its caller says nothing, and MaxLife the
	// longest it may live: a leaked token opens little, for little time.
	DefaultLife = time.Hour
	MaxLife     = 24 * time.Hour
	// secretPrefix marks a gitd token, so that a scanner can tell one when it leaks.
	secretPrefix = "gitd_"
)

// Token is what a token opens, never the token itself: its ID, which can be shown and
// revoked by, the repository it reads, the sandbox repository it writes to when Writable,
// and when it stops opening anything.
type Token struct {
	ID         string
	Repository repo.Repo
	Sandbox    string
	Writable   bool
	Issued     time.Time
	Expires    time.Time
}

// Tokens are the tokens issued and not yet expired or revoked, by the hash of their secret.
type Tokens struct {
	mu     sync.Mutex
	byHash map[string]Token
	now    func() time.Time
}

// NewTokens returns an empty set of tokens.
func NewTokens() *Tokens {
	return &Tokens{byHash: map[string]Token{}, now: time.Now}
}

// Issue makes a token for repository and its sandbox repository, living life (DefaultLife
// when zero, never longer than MaxLife), and answers its secret, shown this once.
func (tokens *Tokens) Issue(repository repo.Repo, sandbox string, writable bool, life time.Duration) (string, Token, error) {
	if life == 0 {
		life = DefaultLife
	}
	if life < time.Minute || life > MaxLife {
		return "", Token{}, git.Fail(git.BadAddress, "a token lives between a minute and %s, not %s", MaxLife, life)
	}
	secret, err := random(32)
	if err != nil {
		return "", Token{}, err
	}
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", Token{}, fmt.Errorf("making a token: %w", err)
	}
	issued := tokens.now()
	token := Token{
		ID: "tok_" + hex.EncodeToString(id), Repository: repository, Sandbox: sandbox,
		Writable: writable, Issued: issued, Expires: issued.Add(life),
	}
	secret = secretPrefix + secret
	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	tokens.byHash[hash(secret)] = token
	return secret, token, nil
}

// Check answers what the token with this secret opens, if it is one and still alive.
func (tokens *Tokens) Check(secret string) (Token, bool) {
	if !strings.HasPrefix(secret, secretPrefix) {
		return Token{}, false
	}
	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	key := hash(secret)
	token, ok := tokens.byHash[key]
	if !ok {
		return Token{}, false
	}
	if !tokens.now().Before(token.Expires) {
		delete(tokens.byHash, key)
		return Token{}, false
	}
	return token, true
}

// Revoke ends the token with this ID, and says whether there was one.
func (tokens *Tokens) Revoke(id string) (Token, bool) {
	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	for key, token := range tokens.byHash {
		if token.ID == id {
			delete(tokens.byHash, key)
			return token, true
		}
	}
	return Token{}, false
}

// RevokeSandbox ends every token of a sandbox repository, as when it is removed.
func (tokens *Tokens) RevokeSandbox(sandbox string) {
	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	for key, token := range tokens.byHash {
		if token.Sandbox == sandbox {
			delete(tokens.byHash, key)
		}
	}
}

// Alive are the tokens still alive, the soonest to expire first; expired ones are
// forgotten on the way.
func (tokens *Tokens) Alive() []Token {
	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	now := tokens.now()
	var alive []Token
	for key, token := range tokens.byHash {
		if !now.Before(token.Expires) {
			delete(tokens.byHash, key)
			continue
		}
		alive = append(alive, token)
	}
	slices.SortFunc(alive, func(first, second Token) int { return first.Expires.Compare(second.Expires) })
	return alive
}

func hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func random(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("making a token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
