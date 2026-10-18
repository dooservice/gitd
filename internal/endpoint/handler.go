package endpoint

import (
	"log/slog"
	"net/http"
	"net/http/cgi"
	"strconv"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// Prefix is where the endpoint answers: `<prefix><host>/<path>.git`, as a sandbox clones it.
const Prefix = "/git/"

// DefaultMaxPush is the most a push may send when the configuration says nothing.
const DefaultMaxPush int64 = 1 << 30

// Handler answers git's smart HTTP for the sandbox repositories, to whoever brings a token:
// the repository in the address must be the token's, and only a token that may write may
// push. Nothing else is served — no dumb protocol, no browsing.
type Handler struct {
	tokens    *Tokens
	sandboxes *Sandboxes
	reading   *cgi.Handler
	writing   *cgi.Handler
}

// NewHandler returns the endpoint over these sandboxes, opened by these tokens, taking
// pushes of at most maxPush bytes.
func NewHandler(runner *git.Runner, tokens *Tokens, sandboxes *Sandboxes, maxPush int64) *Handler {
	if maxPush <= 0 {
		maxPush = DefaultMaxPush
	}
	// Partial clones are offered by each sandbox repository whose pool has the whole
	// history (Sandboxes.Make): against a shallow one git's client loops fetching.
	common := []string{"http.uploadpack=true"}
	return &Handler{
		tokens:    tokens,
		sandboxes: sandboxes,
		reading:   runner.HTTPBackend(sandboxes.Dir, append(common, "http.receivepack=false")),
		writing: runner.HTTPBackend(sandboxes.Dir, append(common, "http.receivepack=true",
			"receive.maxInputSize="+strconv.FormatInt(maxPush, 10))),
	}
}

// Opens says whether secret is a token of this endpoint, so that one listener can serve the
// endpoints of several tenants.
func (handler *Handler) Opens(secret string) bool {
	_, ok := handler.tokens.Check(secret)
	return ok
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	address, service, ok := split(request)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	secret := Secret(request)
	token, opened := handler.tokens.Check(secret)
	if !opened {
		writer.Header().Set("WWW-Authenticate", `Basic realm="gitd"`)
		http.Error(writer, "a gitd token is needed", http.StatusUnauthorized)
		return
	}
	asked, named := repo.Of("https://" + address)
	if !named || asked.HTTPS() != token.Repository.HTTPS() {
		http.NotFound(writer, request)
		return
	}
	backend := handler.reading
	if service == "git-receive-pack" {
		if !token.Writable {
			http.Error(writer, "this token only reads", http.StatusForbidden)
			return
		}
		backend = handler.writing
	}
	slog.Info("endpoint", "token", token.ID, "repository", token.Repository.HTTPS(), "service", service)
	forwarded := request.Clone(request.Context())
	forwarded.Header.Del("Authorization")
	forwarded.URL.Path = "/" + token.Sandbox + ".git/" + strings.TrimPrefix(request.URL.Path[strings.Index(request.URL.Path, ".git/")+len(".git/"):], "/")
	backend.ServeHTTP(writer, forwarded)
}

// split reads the repository and the git service a request is for, and says whether it is
// one of smart HTTP's three requests.
func split(request *http.Request) (string, string, bool) {
	path, ok := strings.CutPrefix(request.URL.Path, Prefix)
	if !ok {
		return "", "", false
	}
	address, rest, ok := strings.Cut(path, ".git/")
	if !ok || address == "" || strings.Contains(address, "..") {
		return "", "", false
	}
	switch rest {
	case "git-upload-pack", "git-receive-pack":
		return address, rest, request.Method == http.MethodPost
	case "info/refs":
		service := request.URL.Query().Get("service")
		return address, service, request.Method == http.MethodGet && (service == "git-upload-pack" || service == "git-receive-pack")
	}
	return "", "", false
}

// Secret is the token a request brings: the password of its basic authentication, as git
// sends what a clone URL carries, or its user when no password came, or a bearer token.
func Secret(request *http.Request) string {
	if bearer, ok := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(bearer)
	}
	user, password, ok := request.BasicAuth()
	if !ok {
		return ""
	}
	if password != "" {
		return password
	}
	return user
}
