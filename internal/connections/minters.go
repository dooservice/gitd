package connections

import (
	"sync"

	"github.com/dooservice/gitd/internal/credentials"
)

// minters are what make the tokens of the connections kept — a GitHub App, a person's
// sign-in — one each, made the first time one is asked for and kept, with the tokens they
// hold, until the connection goes.
type minters struct {
	guard sync.Mutex
	made  map[string]credentials.Minter
}

// minter is the minter of a connection, made by make the first time.
func (store *Store) minter(id string, make func() (credentials.Minter, error)) (credentials.Minter, error) {
	store.minters.guard.Lock()
	defer store.minters.guard.Unlock()
	if made, found := store.minters.made[id]; found {
		return made, nil
	}
	made, err := make()
	if err != nil {
		return nil, err
	}
	if store.minters.made == nil {
		store.minters.made = map[string]credentials.Minter{}
	}
	store.minters.made[id] = made
	return made, nil
}

// forget drops the minter of a connection removed.
func (store *Store) forget(id string) {
	store.minters.guard.Lock()
	defer store.minters.guard.Unlock()
	delete(store.minters.made, id)
}
