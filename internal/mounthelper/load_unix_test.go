//go:build !windows

package mounthelper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAConfigurationAnyoneButRootCouldWriteIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gitd-mount.conf")
	if err := os.WriteFile(path, []byte("user 1500\ntemplates /t\nlayers /l\nroot /r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if os.Geteuid() != 0 && err == nil {
		t.Error("a configuration owned by a user was accepted")
	}
	if os.Geteuid() == 0 {
		if _, err := Load(path); err != nil {
			t.Errorf("root's configuration refused: %v", err)
		}
		if err := os.Chmod(path, 0o666); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Error("a world-writable configuration was accepted")
		}
	}
}

func TestAConfigurationInADirectoryOthersCanWriteIsRefused(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root owns the configuration; runs as root in CI")
	}
	dir := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "gitd-mount.conf")
	if err := os.WriteFile(path, []byte("user 1500\ntemplates /t\nlayers /l\nroot /r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("a configuration in a directory anyone may write was accepted")
	}
}
