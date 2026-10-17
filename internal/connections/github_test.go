package connections

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/secrets"
)

func TestAGitHubAppWhoseKeyCannotBeReadIsNeverKept(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	for name, call := range map[string]func() error{
		"no key":   func() error { _, err := store.AddApp(context.Background(), "", 7, []byte("x"), ""); return err },
		"no ID":    func() error { _, err := store.AddApp(context.Background(), "", 0, pemKey, ""); return err },
		"bad host": func() error { _, err := store.AddApp(context.Background(), "a/b", 7, pemKey, ""); return err },
		"another host": func() error {
			_, err := store.AddApp(context.Background(), "", 7, pemKey, "https://gitlab.com/acme")
			return err
		},
	} {
		if err := call(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(store.List()) != 0 {
		t.Error("an App refused was kept")
	}
}

func TestAGitHubConnectionOpensItsHostOrAPrefixOnIt(t *testing.T) {
	for given, want := range map[string]string{"": "https://github.com", "https://GitHub.com/acme/": "https://github.com/acme"} {
		if got, err := gitHubPrefix("github.com", given); err != nil || got != want {
			t.Errorf("gitHubPrefix(%q) = %q, %v", given, got, err)
		}
	}
}

func TestAGitHubAppIsKeptAsWhatGitHubSaysItIs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/app" || !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"slug": "acme-agents", "html_url": "https://github.com/apps/acme-agents"}`))
	}))
	defer server.Close()
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	store.UseGitHub(GitHub{UserAgent: "gitd/test", APIBase: server.URL})
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	added, err := store.AddApp(context.Background(), "", 7, pemKey, "https://github.com/acme")
	if err != nil {
		t.Fatal(err)
	}
	if added.ActsAs != "acme-agents[bot]" || added.ManageURL != "https://github.com/apps/acme-agents" ||
		added.Host != "github.com" || added.URLPrefix != "https://github.com/acme" {
		t.Errorf("added = %+v", added)
	}
}
