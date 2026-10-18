package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Network is gitd's address on the network, off unless Listen is set: the git endpoint, at
// endpoint.Prefix, served over TLS when a certificate is given. The certificate is read
// again whenever its file changes, so a renewed one needs no restart.
type Network struct {
	Listen      string
	TLSCertFile string
	TLSKeyFile  string
	Git         http.Handler
}

// serveNetwork answers on the network address until ctx ends.
func (server *Server) serveNetwork(ctx context.Context, network *Network) error {
	routes := http.NewServeMux()
	routes.Handle("/git/", network.Git)
	answering := &http.Server{Handler: routes, ReadHeaderTimeout: 10 * time.Second}
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
		listener = tls.NewListener(listener, &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: certificates.get})
	} else {
		slog.Warn("the network address is served without TLS: keep it on a private network", "listen", network.Listen)
	}
	go func() {
		<-ctx.Done()
		stopping, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = answering.Shutdown(stopping)
	}()
	slog.Info("gitd listening on the network", "address", network.Listen, "tls", network.TLSCertFile != "")
	if err := answering.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
