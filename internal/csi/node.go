package csi

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// Workspaces are what the plugin needs of gitd, for one tenant: making a workspace and
// removing it.
type Workspaces interface {
	Prepare(ctx context.Context, request *gitdv1.PrepareRequest) (*gitdv1.WorkspaceResult, error)
	Remove(ctx context.Context, path string) error
}

// Tenant is one tenant's gitd as the plugin reaches it, and the root its workspaces are
// made in; the implicit tenant, when gitd has none, is named "".
type Tenant struct {
	Root       string
	Workspaces Workspaces
}

// Node is the plugin's node service: it publishes an ephemeral volume as a workspace of
// the gitd on its node, and unpublishes it.
type Node struct {
	csi.UnimplementedNodeServer
	NodeID  string
	Tenants map[string]Tenant
	Records Records
}

// NodeGetInfo names the node.
func (node *Node) NodeGetInfo(context.Context, *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{NodeId: node.NodeID}, nil
}

// NodeGetCapabilities offers nothing beyond publishing: no staging, no stats, no expansion.
func (node *Node) NodeGetCapabilities(context.Context, *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{}, nil
}

// NodePublishVolume makes the pod's workspace and mounts it at the volume's target: gitd
// makes it carrying its pool, the pool's objects are mounted read-only inside it, and the
// workspace is bound onto the target. Publishing a volume already published changes nothing.
func (node *Node) NodePublishVolume(ctx context.Context, request *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	target := request.GetTargetPath()
	if request.GetVolumeId() == "" || !filepath.IsAbs(target) {
		return nil, status.Error(codes.InvalidArgument, "a volume ID and an absolute target path are needed")
	}
	if request.GetVolumeCapability().GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "gitd.dev volumes are mounted filesystems, not blocks")
	}
	asked, err := askedOf(request.GetVolumeContext())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	tenant, ok := node.Tenants[asked.Tenant]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "this node serves no tenant %q", asked.Tenant)
	}
	if mounted(target) {
		return &csi.NodePublishVolumeResponse{}, nil
	}
	workspace := asked.workspacePath(tenant.Root)
	if err := node.Records.keep(request.GetVolumeId(), record{Tenant: asked.Tenant, Workspace: workspace}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	prepared := &gitdv1.PrepareRequest{Url: asked.Repository, Path: workspace, PoolInside: true}
	switch {
	case asked.Commit != "":
		prepared.Revision = &gitdv1.Revision{Target: &gitdv1.Revision_Commit{Commit: asked.Commit}}
	case asked.Branch != "":
		prepared.Revision = &gitdv1.Revision{Target: &gitdv1.Revision_Branch{Branch: asked.Branch}}
	}
	made, err := tenant.Workspaces.Prepare(ctx, prepared)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "gitd could not make the workspace: %v", err)
	}
	if err := node.mount(made.GetPoolObjects(), workspace, target, request.GetReadonly()); err != nil {
		node.takeDown(ctx, request.GetVolumeId(), target)
		return nil, status.Error(codes.Internal, err.Error())
	}
	slog.Info("volume published", "volume", request.GetVolumeId(), "workspace", workspace, "repository", asked.Repository)
	return &csi.NodePublishVolumeResponse{}, nil
}

// mount mounts the pool's objects read-only inside the workspace, then the workspace at
// the target.
func (node *Node) mount(objects, workspace, target string, readOnly bool) error {
	inside := filepath.Join(workspace, ".git", "gitd-pool")
	if objects == "" {
		return status.Error(codes.Internal, "gitd named no pool for the workspace to carry")
	}
	if !mounted(inside) {
		if err := directory(inside); err != nil {
			return err
		}
		if err := bind(objects, inside, true, false); err != nil {
			return err
		}
	}
	if err := directory(target); err != nil {
		return err
	}
	return bind(workspace, target, readOnly, true)
}

// NodeUnpublishVolume unmounts the volume and has gitd remove its workspace.
func (node *Node) NodeUnpublishVolume(ctx context.Context, request *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if request.GetVolumeId() == "" || !filepath.IsAbs(request.GetTargetPath()) {
		return nil, status.Error(codes.InvalidArgument, "a volume ID and an absolute target path are needed")
	}
	if err := node.takeDown(ctx, request.GetVolumeId(), request.GetTargetPath()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// takeDown unmounts a volume's target and the pool inside its workspace, and has gitd
// remove the workspace. Taking down what is already down is no error.
func (node *Node) takeDown(ctx context.Context, id, target string) error {
	if err := unmount(target); err != nil {
		return err
	}
	_ = os.Remove(target)
	kept, ok := node.Records.read(id)
	if !ok {
		return nil
	}
	if err := unmount(filepath.Join(kept.Workspace, ".git", "gitd-pool")); err != nil {
		return err
	}
	if tenant, ok := node.Tenants[kept.Tenant]; ok {
		if err := tenant.Workspaces.Remove(ctx, kept.Workspace); err != nil {
			return err
		}
	}
	node.Records.forget(id)
	slog.Info("volume unpublished", "volume", id, "workspace", kept.Workspace)
	return nil
}
