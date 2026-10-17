package connections

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/secrets"
)

func keyedBox(t *testing.T) (*secrets.Box, string) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, key, 0o600); err != nil {
		t.Fatal(err)
	}
	box, err := secrets.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return box, path
}

func TestAConnectionIsKeptAcrossRestartsWithItsSecretSealed(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "connections")
	box, _ := keyedBox(t)
	store, err := Open(directory, box)
	if err != nil {
		t.Fatal(err)
	}
	added, err := store.Add(Record{Kind: Token, URLPrefix: "https://gitlab.example.com/acme"}, []byte("glpat-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(added.ID, "token-") || added.Created.IsZero() {
		t.Errorf("added = %+v", added)
	}
	file := filepath.Join(directory, added.ID+".json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "glpat-secret") {
		t.Error("the secret is on disk as it is, though a key was given")
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("the file's mode is %v", info.Mode().Perm())
	}

	again, err := Open(directory, box)
	if err != nil {
		t.Fatal(err)
	}
	record, secret, err := again.Get(added.ID)
	if err != nil || string(secret) != "glpat-secret" || record.URLPrefix != added.URLPrefix {
		t.Fatalf("after a restart: %+v %q %v", record, secret, err)
	}
}

func TestASecretThatCannotBeOpenedStopsTheStart(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "connections")
	box, _ := keyedBox(t)
	store, err := Open(directory, box)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(Record{Kind: Token, URLPrefix: "https://github.com/acme"}, []byte("token")); err != nil {
		t.Fatal(err)
	}
	other, _ := keyedBox(t)
	if _, err := Open(directory, other); err == nil {
		t.Error("a secret sealed with another key was accepted")
	}
	if _, err := Open(directory, &secrets.Box{}); err == nil {
		t.Error("a sealed secret was accepted without a key")
	}
}

func TestAConnectionIsReplacedAndRemoved(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	added, err := store.Add(Record{Kind: GitHubLogin, URLPrefix: "https://github.com", ActsAs: "josue"}, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(added, []byte("second")); err != nil {
		t.Fatal(err)
	}
	if _, secret, _ := store.Get(added.ID); string(secret) != "second" {
		t.Errorf("secret after replacing = %q", secret)
	}
	if len(store.List()) != 1 {
		t.Errorf("list = %+v", store.List())
	}
	if err := store.Remove(added.ID); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(store.Remove(added.ID), ErrNotFound) || !errors.Is(store.Replace(added, nil), ErrNotFound) {
		t.Error("a removed connection is still known")
	}
	if len(store.List()) != 0 {
		t.Errorf("list after removing = %+v", store.List())
	}
}
