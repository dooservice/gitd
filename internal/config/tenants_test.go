package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loaded(t *testing.T, text string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gitd.toml")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestATenantHasAKeyRootsOfItsOwnAndLimitsGitdCanUse(t *testing.T) {
	settings, err := loaded(t, `
state = "/var/lib/gitd"
[[tenants]]
name = "acme"
api_key_file = "/run/credentials/gitd.service/acme"
roots = ["/var/lib/gitd/tenants-roots/acme"]
limits = { workspaces = 200, disk = "50GB", fetches = 2 }
  [[tenants.credentials]]
  prefix = "https://github.com/acme"
  token_file = "/run/credentials/gitd.service/acme-github"
`)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.HasTenants() {
		t.Fatal("no tenants read")
	}
	acme := settings.ForTenant(settings.Tenants[0])
	if acme.State != "/var/lib/gitd/tenants/acme" || acme.Limits.Fetches != 2 || len(acme.Credentials) != 1 || acme.HasTenants() {
		t.Errorf("acme's configuration = %+v", acme)
	}
	if disk, _ := settings.Tenants[0].Limits.DiskBytes(); disk != 50<<30 {
		t.Errorf("disk = %d", disk)
	}

	for refused, text := range map[string]string{
		"not a tenant name": "[[tenants]]\nname = \"Acme Corp\"\napi_key_file = \"/k\"\nroots = [\"/r\"]\n",
		"api_key_file":      "[[tenants]]\nname = \"acme\"\nroots = [\"/r\"]\n",
		"roots are needed":  "[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\n",
		"written twice":     "[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\nroots = [\"/a\"]\n[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\nroots = [\"/b\"]\n",
		"overlaps":          "[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\nroots = [\"/r\"]\n[[tenants]]\nname = \"globex\"\napi_key_file = \"/k\"\nroots = [\"/r/globex\"]\n",
		"gitd's roots":      "roots = [\"/w\"]\n[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\nroots = [\"/w/acme\"]\n",
		"not a size":        "[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\nroots = [\"/r\"]\nlimits = { disk = \"lots\" }\n",
		"exactly one of":    "[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\nroots = [\"/r\"]\n[[tenants.credentials]]\nprefix = \"https://github.com/acme\"\n",
		"not absolute":      "[[tenants]]\nname = \"acme\"\napi_key_file = \"/k\"\nroots = [\"r\"]\n",
	} {
		if _, err := loaded(t, text); err == nil || !strings.Contains(err.Error(), refused) {
			t.Errorf("%s: %v", refused, err)
		}
	}
}

func TestWithoutTenantsGitdIsOneImplicitTenantAsBefore(t *testing.T) {
	settings, err := loaded(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if settings.HasTenants() {
		t.Error("tenants out of nothing")
	}
}
