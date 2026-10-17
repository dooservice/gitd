package strategy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// copyMounter stands in for the kernel without privilege: "mounting" copies the lower
// layer's files into the target, writes then land in the target only, and unmounting
// empties it. Enough to test everything around a mount.
type copyMounter struct{ fail error }

func (mounter copyMounter) MountOverlay(lower, _, _, target string) error {
	if mounter.fail != nil {
		return mounter.fail
	}
	entries, err := os.ReadDir(lower)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(lower, entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(target, entry.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (copyMounter) Unmount(target string) error {
	entries, _ := os.ReadDir(target)
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(target, entry.Name()))
	}
	return nil
}

func TestAnOverlayCannotNameAPathWithACommaOrAColon(t *testing.T) {
	for _, path := range []string{"/a,b", "/a:b", `/a\b`} {
		if _, err := overlayOptions(path, "/u", "/w"); !errors.Is(err, ErrUnsupported) {
			t.Errorf("overlayOptions(%q) = %v", path, err)
		}
	}
	if options, err := overlayOptions("/l", "/u", "/w"); err != nil || options != "lowerdir=/l,upperdir=/u,workdir=/w" {
		t.Errorf("options = %q, %v", options, err)
	}
}

func TestAnOverlayWorkspaceIsRecordedAndLeavesNothingWhenUnmade(t *testing.T) {
	root := t.TempDir()
	template, dest := filepath.Join(root, "template"), filepath.Join(root, "workspaces", "agent-1")
	if err := os.MkdirAll(template, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(template, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	maker := &OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: copyMounter{}}

	if err := maker.Make(context.Background(), template, dest); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "README.md")); err != nil || string(data) != "hi" {
		t.Errorf("the workspace does not show the template: %q, %v", data, err)
	}
	if records := maker.Records(); len(records) != 1 || records[0] != (Record{Workspace: dest, Template: template}) {
		t.Errorf("Records = %v", records)
	}
	if err := maker.Make(context.Background(), template, dest); err == nil {
		t.Error("made a workspace over one that exists")
	}
	if err := maker.Unmake(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("the workspace's directory is still there")
	}
	if len(maker.Records()) != 0 {
		t.Error("the layer is still there")
	}
}

func TestAMountThatFailsLeavesNothingBehind(t *testing.T) {
	root := t.TempDir()
	maker := &OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: copyMounter{fail: errors.New("permission denied")}}
	dest := filepath.Join(root, "agent-1")
	if err := maker.Make(context.Background(), root, dest); err == nil {
		t.Fatal("a failed mount made a workspace")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("left the workspace's directory")
	}
	if entries, _ := os.ReadDir(maker.Layers); len(entries) != 0 {
		t.Errorf("left a layer: %v", entries)
	}
}

func TestDetectionPicksOverlayWhenItMountsAndCopyWithTheReasonOtherwise(t *testing.T) {
	state := t.TempDir()
	if maker, reason := Detect("", state, nil, copyMounter{}); maker == nil || maker.Name() != Overlay {
		t.Errorf("Detect = %v, %q; want overlay", maker, reason)
	}
	maker, reason := Detect("", state, nil, copyMounter{fail: errors.New("operation not permitted")})
	if maker != nil || reason == "" {
		t.Errorf("Detect = %v, %q; want copy and why", maker, reason)
	}
	if maker, reason := Detect(Copy, state, nil, copyMounter{}); maker != nil || reason != "configured" {
		t.Errorf("Detect(copy) = %v, %q", maker, reason)
	}
	if entries, _ := os.ReadDir(state); len(entries) != 0 {
		t.Errorf("the probes left %v", entries)
	}
}

func TestAReflinkCopyKeepsContentAndModificationTimeOrSaysItCannot(t *testing.T) {
	root := t.TempDir()
	template, dest := filepath.Join(root, "template"), filepath.Join(root, "agent-1")
	if err := os.MkdirAll(filepath.Join(template, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(template, "src", "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("src/main.go", filepath.Join(template, "link")); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}

	err := ReflinkMaker{}.Make(context.Background(), template, dest)
	if errors.Is(err, ErrUnsupported) {
		if _, statErr := os.Stat(dest); statErr == nil {
			t.Error("an unsupported clone left its directory")
		}
		t.Skipf("this filesystem cannot reflink: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dest, "src", "main.go"))
	if err != nil || !info.ModTime().Equal(old) {
		t.Errorf("modification time = %v, %v; want %v", info.ModTime(), err, old)
	}
	if link, err := os.Readlink(filepath.Join(dest, "link")); err != nil || link != "src/main.go" {
		t.Errorf("link = %q, %v", link, err)
	}
}
