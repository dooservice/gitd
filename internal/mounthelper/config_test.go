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
	if config.UID != 1500 || len(config.Templates) != 1 || config.Templates[0] != "/var/lib/gitd/templates" || config.Layers[0] != "/var/lib/gitd/layers" || len(config.Roots) != 2 {
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

func TestEachTenantsDirectoriesMayBeNamed(t *testing.T) {
	config, err := parse(bufio.NewScanner(strings.NewReader(
		"user 1500\ntemplates /var/lib/gitd/tenants/acme/templates\ntemplates /var/lib/gitd/tenants/globex/templates\n" +
			"layers /var/lib/gitd/tenants/acme/layers\nlayers /var/lib/gitd/tenants/globex/layers\n" +
			"root /srv/acme\nroot /srv/globex\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Templates) != 2 || len(config.Layers) != 2 || len(config.Roots) != 2 {
		t.Errorf("config = %+v", config)
	}
}
