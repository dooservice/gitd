package connections

import (
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/secrets"
)

func TestATokenKeptThroughTheAPIOpensItsRepositoriesAtOnce(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := credentials.NewKeys(t.TempDir(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys.AlsoKeep(store)
	repository, _ := repo.Of("https://gitlab.example.com/acme/tools")
	added, err := store.Add(Record{Kind: Token, URLPrefix: "https://gitlab.example.com/acme"}, []byte("glpat"))
	if err != nil {
		t.Fatal(err)
	}
	if !keys.For(repository, nil).Offered {
		t.Fatal("the token kept was not offered")
	}
	if err := store.Remove(added.ID); err != nil {
		t.Fatal(err)
	}
	if keys.For(repository, nil).Offered {
		t.Error("the token removed was still offered")
	}
}
