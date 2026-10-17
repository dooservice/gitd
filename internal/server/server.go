// Package server answers the contract in proto/gitd/v1 over ConnectRPC, on a Unix socket.
//
// Connect speaks gRPC, gRPC-Web and its own HTTP protocol with the same handlers, so the
// client calls it with a generated client and a person can call it with `buf curl`.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/gen/go/gitd/v1/gitdv1connect"
	"github.com/dooservice/gitd/internal/audit"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
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
	// AuditLog is every publish, done or refused.
	AuditLog *audit.Log
}

// Serve answers on the Unix socket until ctx ends, and serves the metrics at
// metricsAddress when there is one.
func (server *Server) Serve(ctx context.Context, socket, metricsAddress string) error {
	routes := http.NewServeMux()
	options := connect.WithInterceptors(&callObserver{metrics: server.Metrics, operation: server.Operation})
	routes.Handle(gitdv1connect.NewWorkspaceServiceHandler(server, options))
	routes.Handle(gitdv1connect.NewRepositoryServiceHandler(server, options))
	routes.Handle(gitdv1connect.NewServerServiceHandler(server, options))

	listener, err := listen(socket)
	if err != nil {
		return err
	}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	answering := &http.Server{Handler: routes, Protocols: protocols, ReadHeaderTimeout: 10 * time.Second}

	if metricsAddress != "" {
		go server.serveMetrics(metricsAddress)
	}
	go func() {
		<-ctx.Done()
		stopping, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = answering.Shutdown(stopping)
	}()
	slog.Info("gitd listening", "socket", socket, "version", server.Version)
	if err := answering.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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

func (server *Server) serveMetrics(address string) {
	handler := promhttp.HandlerFor(server.Metrics.Registry, promhttp.HandlerOpts{})
	metricsServer := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	if err := metricsServer.ListenAndServe(); err != nil {
		slog.Error("metrics not served", "address", address, "error", err)
	}
}
