package githubapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAManifestAsksOnlyForWhatPublishingTakes(t *testing.T) {
	text, err := Manifest("acme-agents", "https://platform.example.com/github/created")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name        string            `json:"name"`
		Redirect    string            `json:"redirect_url"`
		Public      bool              `json:"public"`
		Permissions map[string]string `json:"default_permissions"`
		Hook        struct {
			Active bool `json:"active"`
		} `json:"hook_attributes"`
	}
	if err := json.Unmarshal([]byte(text), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "acme-agents" || manifest.Public || manifest.Hook.Active || len(manifest.Permissions) != 3 ||
		manifest.Permissions["contents"] != "write" || manifest.Permissions["pull_requests"] != "write" {
		t.Errorf("manifest = %+v", manifest)
	}
}

func TestAManifestIsPostedWhereItsOwnerCreatesApps(t *testing.T) {
	if got := FormAction("https://github.com", "acme", "s 1"); got != "https://github.com/organizations/acme/settings/apps/new?state=s+1" {
		t.Errorf("for an organisation: %q", got)
	}
	if got := FormAction("https://github.com", "", "s"); got != "https://github.com/settings/apps/new?state=s" {
		t.Errorf("for a person: %q", got)
	}
	if got := WebBase("github.example.com", 8443); got != "https://github.example.com:8443" {
		t.Errorf("WebBase = %q", got)
	}
}

func TestTheCodeGitHubSendsBackIsExchangedForTheApp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/app-manifests/abc123/conversions" ||
			request.Header.Get("Authorization") != "" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = writer.Write([]byte(`{"id": 9, "slug": "acme-agents", "html_url": "https://github.com/apps/acme-agents",
			"pem": "-----BEGIN RSA PRIVATE KEY-----", "owner": {"login": "acme"}}`))
	}))
	defer server.Close()
	created, err := NewAPI(server.URL, "gitd/test").Convert(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 9 || created.Slug != "acme-agents" || created.Owner.Login != "acme" || created.PEM == "" {
		t.Errorf("created = %+v", created)
	}
}
