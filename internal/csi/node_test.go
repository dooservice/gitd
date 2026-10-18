package csi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// fakeGitd makes a workspace as a directory with a .git, and a pool's objects beside it.
type fakeGitd struct {
	objects string
	removed []string
}

func (fake *fakeGitd) Prepare(_ context.Context, request *gitdv1.PrepareRequest) (*gitdv1.WorkspaceResult, error) {
	if err := os.MkdirAll(filepath.Join(request.GetPath(), ".git"), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(request.GetPath(), "README.md"), []byte("hi"), 0o644); err != nil {
		return nil, err
	}
	return &gitdv1.WorkspaceResult{PoolObjects: fake.objects}, nil
}

func (fake *fakeGitd) Remove(_ context.Context, path string) error {
	fake.removed = append(fake.removed, path)
	return os.RemoveAll(path)
}

func podContext(extra map[string]string) map[string]string {
	attributes := map[string]string{
		contextEphemeral: "true", contextNamespace: "agents", contextPod: "agent-1",
		"repository": "https://github.com/acme/web",
	}
	for key, value := range extra {
		attributes[key] = value
	}
	return attributes
}

func TestAVolumeAsksForARepositoryForOnePodOnly(t *testing.T) {
	asked, err := askedOf(podContext(map[string]string{"branch": "main", "name": "web"}))
	if err != nil {
		t.Fatal(err)
	}
	if path := asked.workspacePath("/var/lib/gitd/workspaces"); path != "/var/lib/gitd/workspaces/agents/agent-1-web" {
		t.Errorf("path = %s", path)
	}
	for why, attributes := range map[string]map[string]string{
		"not ephemeral":     podContext(map[string]string{contextEphemeral: "false"}),
		"no repository":     podContext(map[string]string{"repository": ""}),
		"branch and commit": podContext(map[string]string{"branch": "main", "commit": "abc"}),
		"no pod":            podContext(map[string]string{contextPod: ""}),
		"a path for a pod":  podContext(map[string]string{contextPod: "../etc"}),
		"a path for a name": podContext(map[string]string{"name": "../x"}),
	} {
		if _, err := askedOf(attributes); err == nil {
			t.Errorf("%s: accepted", why)
		}
	}
	if _, err := (Records{Dir: t.TempDir()}).path("../escape"); err == nil {
		t.Error("a volume ID made a path out of the records")
	}
}

func TestAPodsVolumeIsItsWorkspaceWithThePoolInsideAndGoesWithIt(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("mounting takes root")
	}
	base := t.TempDir()
	objects := filepath.Join(base, "pools", "web.git", "objects")
	if err := os.MkdirAll(objects, 0o755); err != nil {
		t.Fatal(err)
	}
	gitd := &fakeGitd{objects: objects}
	node := &Node{NodeID: "node-1", Tenants: map[string]Tenant{"": {Root: filepath.Join(base, "workspaces"), Workspaces: gitd}},
		Records: Records{Dir: filepath.Join(base, "records")}}
	target := filepath.Join(base, "pods", "uid", "volumes", "gitd")
	request := &csi.NodePublishVolumeRequest{
		VolumeId: "csi-0123", TargetPath: target, VolumeContext: podContext(nil),
		VolumeCapability: &csi.VolumeCapability{AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}}},
	}
	ctx := context.Background()
	if _, err := node.NodePublishVolume(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := node.NodePublishVolume(ctx, request); err != nil {
		t.Errorf("publishing again: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(target, "README.md")); err != nil || string(data) != "hi" {
		t.Errorf("the pod does not see its workspace: %q, %v", data, err)
	}
	inside := filepath.Join(target, ".git", "gitd-pool")
	if !mounted(filepath.Join(base, "workspaces", "agents", "agent-1", ".git", "gitd-pool")) {
		t.Error("the pool is not mounted inside the workspace")
	}
	if err := os.WriteFile(filepath.Join(inside, "x"), nil, 0o644); err == nil {
		t.Error("the pool inside the workspace is writable")
	}
	if _, err := node.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{VolumeId: "csi-0123", TargetPath: target}); err != nil {
		t.Fatal(err)
	}
	if mounted(target) || len(gitd.removed) != 1 {
		t.Errorf("unpublished: target mounted %v, removed %v", mounted(target), gitd.removed)
	}
	if entries, _ := os.ReadDir(objects); len(entries) != 0 {
		t.Error("removing the workspace reached into the pool")
	}
	if _, err := node.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{VolumeId: "csi-0123", TargetPath: target}); err != nil {
		t.Errorf("unpublishing again: %v", err)
	}
}
