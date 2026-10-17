package connections

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
)

func TestTheFilesGitHubAppsAreCredentialsAndShown(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyFile := filepath.Join(t.TempDir(), "app.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	declared := []config.Credential{
		{Prefix: "https://github.com/acme", TokenFile: "/run/token"},
		{Prefix: "https://github.com/acme/tools", GitHubApp: &config.GitHubApp{AppID: 7, PrivateKeyFile: keyFile}},
	}
	apps, err := NewFileApps(declared, GitHub{UserAgent: "gitd/test"})
	if err != nil {
		t.Fatal(err)
	}
	kept := apps.Credentials()
	if len(kept) != 1 || kept[0].Minter == nil || kept[0].Prefix != "https://github.com/acme/tools" {
		t.Errorf("credentials = %+v", kept)
	}
	shown := ShownFromFile(declared)
	if shown[1].GetKind() != gitdv1.Connection_KIND_GITHUB_APP || shown[1].GetAppId() != 7 {
		t.Errorf("shown = %v", shown)
	}
	missing := []config.Credential{{Prefix: "https://github.com/acme", GitHubApp: &config.GitHubApp{AppID: 7, PrivateKeyFile: "/missing"}}}
	if _, err := NewFileApps(missing, GitHub{}); err == nil {
		t.Error("an App whose key cannot be read was accepted")
	}
}
