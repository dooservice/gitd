//go:build linux

package csi

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

// Serve answers kubelet on the Unix socket at path until ctx ends.
func Serve(ctx context.Context, path string, identity *Identity, node *Node) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	csi.RegisterIdentityServer(server, identity)
	csi.RegisterNodeServer(server, node)
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	return server.Serve(listener)
}
