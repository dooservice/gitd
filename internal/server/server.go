// Package server answers the contract in proto/gitd/v1 over ConnectRPC, on a Unix socket.
//
// Connect speaks gRPC, gRPC-Web and its own HTTP protocol with the same handlers, so the
// client calls it with a generated client and a person can call it with `buf curl`.
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/gen/go/gitd/v1/gitdv1connect"
	"github.com/dooservice/gitd/internal/audit"
	"github.com/dooservice/gitd/internal/connections"
	"github.com/dooservice/gitd/internal/events"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/workspace"
)

// Server is the handlers of every service in the contract. Operation is how long a call
// may take when its caller set no deadline, and Totals says how many pools there are and
// the disk they take.
type Server struct {
	Version     string
	Workspaces  *workspace.Workspaces
	Pools       *pool.Pools
	Runner      *git.Runner
	Metrics     *metrics.Metrics
	Operation   time.Duration
	Totals      func() (int, int64)
	RetirePools func(context.Context, time.Duration) (int, int64)
	// Strategy is how new workspaces are made here, and StrategyReason why, as detected at
	// start; TemplateTotals how many templates there are and the disk they take.
	Strategy       gitdv1.Strategy
	StrategyReason string
	TemplateTotals func() (int, int64)
	// AuditLog is every publish, done or refused, and every change to connections and
	// publishing settings.
	AuditLog *audit.Log
	// Connections are the credentials kept through the API, and FileConnections those the
	// configuration file declares, as the API shows them.
	Connections     *connections.Store
	FileConnections []*gitdv1.Connection
	// Publishing is the publishing settings, the file's and those set through the API.
	Publishing *publishing.Store
	// Events carry what happens to workspaces to Watch.
	Events *events.Bus
	// Endpoint is the git endpoint's tokens and sandbox repositories, when one is served.
	Endpoint *Endpoint
}

// Routes are the handlers of every service this part of gitd answers, one tenant's or the
// implicit one's.
func (server *Server) Routes() http.Handler {
	routes := http.NewServeMux()
	options := connect.WithInterceptors(&callObserver{metrics: server.Metrics, operation: server.Operation})
	routes.Handle(gitdv1connect.NewWorkspaceServiceHandler(server, options))
	routes.Handle(gitdv1connect.NewRepositoryServiceHandler(server, options))
	routes.Handle(gitdv1connect.NewServerServiceHandler(server, options))
	if server.Connections != nil {
		routes.Handle(gitdv1connect.NewConnectionServiceHandler(&connectionHandlers{server: server}, options))
	}
	if server.Publishing != nil {
		routes.Handle(gitdv1connect.NewPublishingServiceHandler(&publishingHandlers{server: server}, options))
	}
	if server.Endpoint != nil {
		routes.Handle(gitdv1connect.NewEndpointServiceHandler(&endpointHandlers{server: server}, options))
	}
	return routes
}

// listen opens the socket, replacing one a previous gitd left, readable by its group only.
func listen(socket string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	return listener, os.Chmod(socket, 0o660)
}
