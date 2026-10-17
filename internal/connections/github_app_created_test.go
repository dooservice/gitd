package connections

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/secrets"
)

func TestAnAppIsCreatedFromAManifestOnceAndKept(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/app-manifests/code-1/conversions" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = writer.Write([]byte(`{"id": 9, "slug": "acme-agents", "html_url": "https://github.com/apps/acme-agents",
			"pem": "-----BEGIN RSA PRIVATE KEY-----", "owner": {"login": "acme"}}`))
	}))
	defer github.Close()
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	store.UseGitHub(GitHub{UserAgent: "gitd/test", APIBase: github.URL, WebBase: "https://github.com"})

	toCreate, err := store.BeginApp("", "acme", "acme-agents", "https://platform.example.com/github/created")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(toCreate.FormAction, "https://github.com/organizations/acme/settings/apps/new?state=") ||
		!strings.Contains(toCreate.Manifest, `"name":"acme-agents"`) || len(toCreate.State) < 32 {
		t.Fatalf("to create = %+v", toCreate)
	}
	record, install, err := store.CompleteApp(context.Background(), "code-1", toCreate.State)
	if err != nil {
		t.Fatal(err)
	}
	if record.AppID != 9 || record.ActsAs != "acme-agents[bot]" || record.URLPrefix != "https://github.com/acme" ||
		install != "https://github.com/apps/acme-agents/installations/new" {
		t.Errorf("record = %+v, install %q", record, install)
	}
	if _, key, _ := store.Get(record.ID); string(key) != "-----BEGIN RSA PRIVATE KEY-----" {
		t.Error("the App's key was not kept")
	}
	if _, _, err := store.CompleteApp(context.Background(), "code-1", toCreate.State); !errors.Is(err, ErrInvalid) {
		t.Errorf("a state used twice: %v", err)
	}
}

func TestAnAppToCreateIsCheckedBeforeGitHubSeesIt(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	for name, values := range map[string][3]string{
		"bad organisation": {"acme/x", "agents", "https://platform.example.com/cb"},
		"long name":        {"acme", strings.Repeat("a", 40), "https://platform.example.com/cb"},
		"plain http":       {"acme", "agents", "http://platform.example.com/cb"},
		"no address":       {"acme", "agents", "nowhere"},
	} {
		if _, err := store.BeginApp("", values[0], values[1], values[2]); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := store.BeginApp("", "", "agents", "http://localhost:4242/cb"); err != nil {
		t.Errorf("a command line waiting on localhost: %v", err)
	}
	if _, _, err := store.CompleteApp(context.Background(), "code", "never-given"); !errors.Is(err, ErrInvalid) {
		t.Errorf("a state never given: %v", err)
	}
}
