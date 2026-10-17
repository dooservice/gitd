package connections

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/secrets"
)

func TestAGitHubAppKeptMakesItsTokensAsACredentialUntilItIsRemoved(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	store.UseGitHub(GitHub{UserAgent: "gitd/test"})
	added, err := store.Add(Record{Kind: GitHubApp, URLPrefix: "https://github.com/acme", Host: "github.com", AppID: 7}, pemKey)
	if err != nil {
		t.Fatal(err)
	}
	kept := store.Credentials()
	if len(kept) != 1 || kept[0].Minter == nil || kept[0].Label != "GitHub App 7" || kept[0].Prefix != "https://github.com/acme" {
		t.Fatalf("credentials = %+v", kept)
	}
	if store.Credentials()[0].Minter != kept[0].Minter {
		t.Error("the App was made again, losing the tokens it kept")
	}
	if err := store.Remove(added.ID); err != nil {
		t.Fatal(err)
	}
	if len(store.Credentials()) != 0 || len(store.apps.made) != 0 {
		t.Error("the App removed still makes tokens")
	}
}
