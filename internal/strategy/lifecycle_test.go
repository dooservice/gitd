package strategy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// countingMounter "mounts" as copyMounter does, counting.
type countingMounter struct {
	copyMounter
	mounts int
}

func (mounter *countingMounter) MountOverlay(lowers []string, upper, work, target string) error {
	mounter.mounts++
	return mounter.copyMounter.MountOverlay(lowers, upper, work, target)
}

func TestARestartMountsAgainWhatIsUnmountedAndNeverMountsOverFiles(t *testing.T) {
	root := t.TempDir()
	template := filepath.Join(root, "template")
	if err := os.MkdirAll(template, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(template, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mounter := &countingMounter{}
	maker := &OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: mounter}
	ctx := context.Background()
	for _, name := range []string{"rebooted", "written-into", "gone"} {
		if err := maker.Make(ctx, template, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
		_ = mounter.Unmount(filepath.Join(root, name)) // as a reboot leaves them
	}
	if err := os.WriteFile(filepath.Join(root, "written-into", "stray"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "gone")); err != nil {
		t.Fatal(err)
	}
	mounter.mounts = 0

	remounted, failed := maker.Remount(ctx)
	if remounted != 1 || mounter.mounts != 1 {
		t.Errorf("remounted %d (%d mounts); want the rebooted one only", remounted, mounter.mounts)
	}
	if len(failed) != 1 {
		t.Errorf("failed = %v; want the one written into", failed)
	}
	if data, err := os.ReadFile(filepath.Join(root, "rebooted", "README.md")); err != nil || string(data) != "hi" {
		t.Errorf("the rebooted workspace does not show its template: %q, %v", data, err)
	}

	removed := maker.Sweep()
	if len(removed) != 1 || removed[0].Workspace != filepath.Join(root, "gone") {
		t.Errorf("swept %v; want the gone workspace's layer", removed)
	}
	if records := maker.Records(); len(records) != 2 {
		t.Errorf("layers left = %v; want the rebooted and the written-into ones", records)
	}
}

func TestAHalfBuiltLayerIsSwept(t *testing.T) {
	layers := t.TempDir()
	if err := os.MkdirAll(filepath.Join(layers, "abc", "upper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if removed := (&OverlayMaker{Layers: layers}).Sweep(); len(removed) != 0 {
		t.Errorf("a half-built layer was reported as a workspace's: %v", removed)
	}
	if _, err := os.Stat(filepath.Join(layers, "abc")); err == nil {
		t.Error("the half-built layer is still there")
	}
}

func TestALiftedLayerLeavesItsGitBehindAndIsPutBackWhenNotTaken(t *testing.T) {
	root := t.TempDir()
	template, dest := filepath.Join(root, "template"), filepath.Join(root, "workspaces", "agent-1")
	if err := os.MkdirAll(template, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(template, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mounter := &countingMounter{}
	maker := &OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: mounter}
	if err := maker.Make(context.Background(), template, dest); err != nil {
		t.Fatal(err)
	}
	_, upper, _, _ := maker.LayersOf(dest)
	if err := os.MkdirAll(filepath.Join(upper, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	lifted, err := maker.Lift(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lifted.Upper, ".git")); !os.IsNotExist(err) {
		t.Errorf("the workspace's .git is still in the lifted layer: %v", err)
	}
	if err := lifted.Restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(upper, ".git")); err != nil || mounter.mounts != 2 {
		t.Errorf("restored: .git %v, %d mounts", err, mounter.mounts)
	}
	lifted, err = maker.Lift(dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(lifted.Upper, filepath.Join(root, "sealed")); err != nil {
		t.Fatal(err)
	}
	lifted.Drop()
	if _, err := os.Stat(dest); !os.IsNotExist(err) || len(maker.Records()) != 0 {
		t.Errorf("dropped: %v, records %v", err, maker.Records())
	}
}

func TestARestartMountsAWorkspaceAgainOnItsWarmLayer(t *testing.T) {
	root := t.TempDir()
	template, warm, dest := filepath.Join(root, "template"), filepath.Join(root, "warm"), filepath.Join(root, "agent-1")
	for dir, content := range map[string]string{template: "template", warm: "warm"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mounter := &countingMounter{}
	maker := &OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: mounter}
	if err := maker.MakeWarm(context.Background(), template, warm, dest); err != nil {
		t.Fatal(err)
	}
	if record, ok := maker.RecordOf(dest); !ok || record.Warm != warm || record.Template != template {
		t.Errorf("record = %+v, %v", record, ok)
	}
	_ = mounter.Unmount(dest)
	if remounted, failed := maker.Remount(context.Background()); remounted != 1 || len(failed) != 0 {
		t.Fatalf("remounted %d, failed %v", remounted, failed)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "README.md")); string(data) != "warm" {
		t.Errorf("remounted without its warm layer: README.md is %q", data)
	}
}
