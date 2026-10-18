//go:build linux

// Command gitd-csi is gitd's Kubernetes CSI node plugin (internal/csi): a pod's ephemeral
// volume of the driver gitd.dev becomes a workspace of the gitd on its node, with no
// credential in the pod. It runs privileged, beside gitd, as the Helm chart deploys it.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dooservice/gitd/internal/csi"
)

var version = "dev"

// repeated is a flag given any number of times.
type repeated []string

func (values *repeated) String() string         { return strings.Join(*values, " ") }
func (values *repeated) Set(value string) error { *values = append(*values, value); return nil }

func main() {
	endpoint := flag.String("endpoint", "/csi/csi.sock", "the socket kubelet calls the plugin on")
	nodeID := flag.String("node-id", os.Getenv("NODE_NAME"), "this node's name")
	socket := flag.String("gitd-socket", "/run/gitd/gitd.sock", "the socket of the gitd on this node")
	root := flag.String("root", "/var/lib/gitd/workspaces", "where workspaces are made, when gitd has no tenants")
	records := flag.String("records", "/var/lib/gitd/csi", "where the plugin keeps the volumes it published")
	var tenants repeated
	flag.Var(&tenants, "tenant", "a tenant the plugin serves: name=root,keyfile (repeat for each)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	served, err := csi.TenantsOf(*socket, *root, tenants)
	if err != nil {
		slog.Error("gitd-csi stopped", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	identity := &csi.Identity{Version: version, Ready: csi.Ready(served)}
	node := &csi.Node{NodeID: *nodeID, Tenants: served, Records: csi.Records{Dir: *records}}
	slog.Info("gitd-csi answering", "endpoint", *endpoint, "node", *nodeID, "tenants", len(served))
	if err := csi.Serve(ctx, *endpoint, identity, node); err != nil {
		slog.Error("gitd-csi stopped", "error", err)
		os.Exit(1)
	}
}
