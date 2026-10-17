package mounthelper

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// These mount: they need root, and CI runs them with sudo.
func TestGitdMountMountsAndUnmountsAnOverlayAndNothingElse(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("mounting needs root")
	}
	base := t.TempDir()
	config := Config{UID: 0, Templates: filepath.Join(base, "templates"), Layers: filepath.Join(base, "layers"), Roots: []string{filepath.Join(base, "workspaces")}}
	lower := filepath.Join(config.Templates, "web")
	upper, work := filepath.Join(config.Layers, "1", "upper"), filepath.Join(config.Layers, "1", "work")
	target := filepath.Join(config.Roots[0], "agent-1")
	for _, dir := range []string{lower, upper, work, target} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(lower, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(base, "gitd-mount.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { _ = Serve(listener, config) }()
	client := Client{Socket: socket}

	if err := client.MountOverlay([]string{lower}, upper, work, target); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(target, "README.md")); err != nil || string(data) != "hi" {
		t.Errorf("the overlay does not show the template: %q, %v", data, err)
	}
	if err := client.MountOverlay([]string{lower}, upper, work, target); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("mounted over a mounted workspace: %v", err)
	}
	if err := client.Unmount(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "README.md")); err == nil {
		t.Error("still mounted after unmounting")
	}
	if err := client.Unmount(target); err != nil {
		t.Errorf("unmounting what is not mounted: %v", err)
	}

	// A warm layer stands between the template and the workspace's own layer, and wins.
	warm := filepath.Join(config.Templates, ".warm", "web")
	if err := os.MkdirAll(warm, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(warm, "README.md"), []byte("warm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.MountOverlay([]string{warm, lower}, upper, work, target); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(target, "README.md")); err != nil || string(data) != "warm" {
		t.Errorf("the overlay does not show the warm layer: %q, %v", data, err)
	}
	if err := client.Unmount(target); err != nil {
		t.Fatal(err)
	}
	// Each lower layer is checked on its own, and there are at most two.
	if err := client.MountOverlay([]string{upper, lower}, upper, work, target); err == nil {
		t.Error("mounted a lower layer outside the templates")
	}
	if err := client.MountOverlay([]string{warm, warm, lower}, upper, work, target); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Errorf("mounted three lower layers: %v", err)
	}

	// A mount that is not an overlay is never unmounted.
	other := filepath.Join(config.Roots[0], "agent-2")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount("tmpfs", other, "tmpfs", 0, ""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Unmount(other, 0) }()
	if err := client.Unmount(other); err == nil || !strings.Contains(err.Error(), "not an overlay") {
		t.Errorf("unmounted a tmpfs: %v", err)
	}
}
