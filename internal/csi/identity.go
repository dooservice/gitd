// Package csi is gitd's Kubernetes CSI node plugin: a pod asks for a workspace like any
// volume — an ephemeral inline volume of the driver gitd.dev — and gets a copy-on-write
// checkout, made by the gitd on its node, with no credential in the pod.
//
// The plugin asks gitd for a workspace that carries its pool inside it (pool_inside),
// mounts the pool's objects read-only at the workspace's .git/gitd-pool, and binds the
// workspace onto the pod's volume. When the pod goes it unmounts both and gitd removes the
// workspace. CSI node plugins are privileged by design: that is where the privilege to mount
// belongs, not in gitd, nor in the pod.
package csi

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// DriverName is the name pods ask for: `csi: { driver: gitd.dev }`.
const DriverName = "gitd.dev"

// Identity answers who the plugin is, and whether the gitd it needs answers.
type Identity struct {
	csi.UnimplementedIdentityServer
	Version string
	// Ready says whether gitd answers.
	Ready func(context.Context) error
}

// GetPluginInfo names the driver.
func (identity *Identity) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{Name: DriverName, VendorVersion: identity.Version}, nil
}

// GetPluginCapabilities offers nothing beyond a node plugin: no controller, no topology.
func (identity *Identity) GetPluginCapabilities(context.Context, *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{}, nil
}

// Probe says the plugin is ready once gitd answers.
func (identity *Identity) Probe(ctx context.Context, _ *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	if identity.Ready != nil {
		if err := identity.Ready(ctx); err != nil {
			return &csi.ProbeResponse{Ready: wrapperspb.Bool(false)}, nil
		}
	}
	return &csi.ProbeResponse{Ready: wrapperspb.Bool(true)}, nil
}
