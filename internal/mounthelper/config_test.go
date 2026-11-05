//go:build !windows

package mounthelper

import (
	"bufio"
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
