package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/dooservice/gitd/internal/apikeys"
	"github.com/dooservice/gitd/internal/endpoint"
	"github.com/dooservice/gitd/internal/metrics"
)

// Tenant is one part of gitd behind the gateway: a tenant's, or the implicit one when
// there are none. Key is what its callers bring; API its services; Git its endpoint, if any.
type Tenant struct {
	Name string
	Key  *apikeys.File
	API  http.Handler
	Git  *endpoint.Handler
}

// Network is gitd's address on the network, off unless Listen is set, served over TLS when
// a certificate is given; a renewed certificate is read without a restart.
type Network struct {
	Listen      string
	TLSCertFile string
	TLSKeyFile  string
}

// Gateway is where gitd answers: its Unix socket, its network address, and its metrics.
// Every call reaches one part of gitd only. Without tenants that is the implicit part,
// which the socket answers as before and the network only with its key. With tenants, a
// call's key names its tenant — on the socket too — and a call without a valid key is
// refused; an endpoint token belongs to one tenant.
type Gateway struct {
	Version        string
	Socket         string
	MetricsAddress string
	Metrics        *metrics.Metrics
	Network        *Network
	Implicit       *Tenant
	Tenants        []*Tenant
}

// Serve answers until ctx ends.
func (gateway *Gateway) Serve(ctx context.Context) error {
	listener, err := listen(gateway.Socket)
	if err != nil {
		return err
	}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	onSocket := &http.Server{Handler: http.HandlerFunc(gateway.socket), Protocols: protocols, ReadHeaderTimeout: 10 * time.Second}
	stopOnDone(ctx, onSocket)
	if gateway.MetricsAddress != "" {
		go gateway.serveMetrics()
	}
	if gateway.Network != nil {
		failed := make(chan error, 1)
		go func() { failed <- gateway.serveNetwork(ctx) }()
		select {
		case err := <-failed:
			if err != nil {
				_ = listener.Close()
				return err
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	slog.Info("gitd listening", "socket", gateway.Socket, "version", gateway.Version, "tenants", len(gateway.Tenants))
	if err := onSocket.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// socket answers a call on the Unix socket: the implicit part's with no key, as before
// tenants, or the tenant the key names.
func (gateway *Gateway) socket(writer http.ResponseWriter, request *http.Request) {
	if gateway.Implicit != nil {
		gateway.Implicit.API.ServeHTTP(writer, request)
		return
	}
	gateway.byKey(writer, request)
}

// byKey answers a call as the part of gitd its key names, or refuses it.
func (gateway *Gateway) byKey(writer http.ResponseWriter, request *http.Request) {
	key := apikeys.Of(request)
	for _, tenant := range gateway.parts() {
		if tenant.Key != nil && tenant.Key.Matches(key) {
			tenant.API.ServeHTTP(writer, request)
			return
		}
	}
	writer.Header().Set("WWW-Authenticate", `Bearer realm="gitd"`)
	http.Error(writer, "a valid key is needed: Authorization: Bearer <key>", http.StatusUnauthorized)
}

// network answers on the network address: the endpoint, to whoever brings a token of one
// of its parts, and the API, to whoever brings a key.
func (gateway *Gateway) network(writer http.ResponseWriter, request *http.Request) {
	if len(request.URL.Path) >= len(endpoint.Prefix) && request.URL.Path[:len(endpoint.Prefix)] == endpoint.Prefix {
		secret := endpoint.Secret(request)
		for _, tenant := range gateway.parts() {
			if tenant.Git != nil && tenant.Git.Opens(secret) {
				tenant.Git.ServeHTTP(writer, request)
				return
			}
		}
		writer.Header().Set("WWW-Authenticate", `Basic realm="gitd"`)
		http.Error(writer, "a gitd token is needed", http.StatusUnauthorized)
		return
	}
	gateway.byKey(writer, request)
}

func (gateway *Gateway) parts() []*Tenant {
	if gateway.Implicit != nil {
		return []*Tenant{gateway.Implicit}
	}
	return gateway.Tenants
}

func (gateway *Gateway) serveNetwork(ctx context.Context) error {
	network := gateway.Network
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	answering := &http.Server{
		Handler: http.HandlerFunc(gateway.network), Protocols: protocols,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute,
	}
	listener, err := net.Listen("tcp", network.Listen)
	if err != nil {
		return err
	}
	if network.TLSCertFile != "" {
		certificates := &certificateFiles{cert: network.TLSCertFile, key: network.TLSKeyFile}
		if _, err := certificates.get(nil); err != nil {
			_ = listener.Close()
			return err
		}
		listener = tls.NewListener(listener, &tls.Config{
			MinVersion: tls.VersionTLS12, GetCertificate: certificates.get, NextProtos: []string{"h2", "http/1.1"},
		})
	} else {
		slog.Warn("the network address is served without TLS: keep it on a private network", "listen", network.Listen)
	}
	stopOnDone(ctx, answering)
	slog.Info("gitd listening on the network", "address", network.Listen, "tls", network.TLSCertFile != "")
	if err := answering.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (gateway *Gateway) serveMetrics() {
	handler := promhttp.HandlerFor(gateway.Metrics.Registry, promhttp.HandlerOpts{})
	metricsServer := &http.Server{Addr: gateway.MetricsAddress, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	if err := metricsServer.ListenAndServe(); err != nil {
		slog.Error("metrics not served", "address", gateway.MetricsAddress, "error", err)
	}
}

// stopOnDone shuts answering down when ctx ends, giving calls half a minute to finish.
func stopOnDone(ctx context.Context, answering *http.Server) {
	go func() {
		<-ctx.Done()
		stopping, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = answering.Shutdown(stopping)
	}()
}
