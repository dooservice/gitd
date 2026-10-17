package mounthelper

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheConfigurationNamesAUserAndCleanDirectories(t *testing.T) {
	config, err := parse(bufio.NewScanner(strings.NewReader(`
# who and where
user 1500
templates /var/lib/gitd/templates
layers /var/lib/gitd/layers
root /var/lib/gitd/workspaces
root /srv/agents
`)))
	if err != nil {
		t.Fatal(err)
	}
	if config.UID != 1500 || config.Templates != "/var/lib/gitd/templates" || config.Layers != "/var/lib/gitd/layers" || len(config.Roots) != 2 {
		t.Errorf("config = %+v", config)
	}
	for _, text := range []string{
		"user 0\ntemplates /t\nlayers /l\nroot /r\n",            // root may not be the caller
		"user 1500\ntemplates t\nlayers /l\nroot /r\n",          // relative
		"user 1500\ntemplates /t/../etc\nlayers /l\nroot /r\n",  // not clean
		"user 1500\ntemplates /t\nlayers /l\nroot /\n",          // the whole disk
		"user 1500\ntemplates /t\nlayers /l\n",                  // no root
		"user 1500\ntemplates /t\nlayers /l\nroot /r\nwide /\n", // unknown key
	} {
		if _, err := parse(bufio.NewScanner(strings.NewReader(text))); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}

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
