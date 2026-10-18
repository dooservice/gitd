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
	maxPush   int64
	// slots bound how many git processes the endpoint runs at once; a request finding none
	// free is told to come back.
	slots chan struct{}
	// limits, when set, refuse a push once the tenant's disk is full.
	limits interface{ WithinDisk() error }
}

// concurrent is how many git processes one tenant's endpoint runs at once.
const concurrent = 16

// maxFetchRequest bounds what a clone or fetch may send: its wants and haves.
const maxFetchRequest = 16 << 20

// NewHandler returns the endpoint over these sandboxes, opened by these tokens, taking
// pushes of at most maxPush bytes.
func NewHandler(runner *git.Runner, tokens *Tokens, sandboxes *Sandboxes, maxPush int64) *Handler {
	if maxPush <= 0 {
		maxPush = DefaultMaxPush
	}
	// Partial clones are offered by each sandbox repository whose pool has the whole
	// history (Sandboxes.Make): against a shallow one git's client loops fetching.
	//
	// No ref of the pool is ever shown: receive-pack would offer the tips of every ref of the
	// repositories a sandbox repository borrows from, the pool's own among them — commits of
	// other sandboxes and workspaces staged and not published — and a push naming one would
	// take it.
	common := []string{"http.uploadpack=true", "core.alternateRefsCommand=true"}
	return &Handler{
		tokens:    tokens,
		sandboxes: sandboxes,
		reading:   runner.HTTPBackend(sandboxes.Dir, append(common, "http.receivepack=false")),
		writing: runner.HTTPBackend(sandboxes.Dir, append(common, "http.receivepack=true",
			"receive.maxInputSize="+strconv.FormatInt(maxPush, 10))),
		maxPush: maxPush,
		slots:   make(chan struct{}, concurrent),
	}
}

// UseLimits refuses pushes once limits say the tenant's disk is full.
func (handler *Handler) UseLimits(limits interface{ WithinDisk() error }) {
	handler.limits = limits
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
	backend, most := handler.reading, int64(maxFetchRequest)
	if service == "git-receive-pack" {
		if !token.Writable {
			http.Error(writer, "this token only reads", http.StatusForbidden)
			return
		}
		if handler.limits != nil {
			if err := handler.limits.WithinDisk(); err != nil {
				http.Error(writer, err.Error(), http.StatusInsufficientStorage)
				return
			}
		}
		backend, most = handler.writing, handler.maxPush+maxFetchRequest
	}
	select {
	case handler.slots <- struct{}{}:
		defer func() { <-handler.slots }()
	default:
		writer.Header().Set("Retry-After", "5")
		http.Error(writer, "too many git requests at once: try again", http.StatusServiceUnavailable)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, most)
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
		// One service, named once: git's http-backend reads the last of several, Go the first.
		services := request.URL.Query()["service"]
		if len(services) != 1 {
			return "", "", false
		}
		service := services[0]
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
