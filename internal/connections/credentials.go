package connections

import "github.com/dooservice/gitd/internal/credentials"

// Credentials are what the connections kept open repositories with, as gitd asks for them
// at every operation: so a connection added or removed applies at once.
func (store *Store) Credentials() []credentials.Credential {
	store.guard.RLock()
	defer store.guard.RUnlock()
	kept := make([]credentials.Credential, 0, len(store.records))
	for id, record := range store.records {
		if record.Kind == Token {
			kept = append(kept, credentials.KeptToken(record.URLPrefix, string(store.opened[id]), record.User))
		}
	}
	return kept
}
