package connections

import (
	"log/slog"

	"github.com/dooservice/gitd/internal/credentials"
)

// Credentials are what the connections kept open repositories with, as gitd asks for them
// at every operation: so a connection added or removed applies at once. A GitHub App makes
// a token for each repository and purpose.
func (store *Store) Credentials() []credentials.Credential {
	store.guard.RLock()
	records := make([]Record, 0, len(store.records))
	tokens := map[string]string{}
	for id, record := range store.records {
		records = append(records, record)
		if record.Kind == Token {
			tokens[id] = string(store.opened[id])
		}
	}
	store.guard.RUnlock()

	kept := make([]credentials.Credential, 0, len(records))
	for _, record := range records {
		switch record.Kind {
		case Token:
			kept = append(kept, credentials.KeptToken(record.URLPrefix, tokens[record.ID], record.User))
		case GitHubApp:
			app, err := store.app(record)
			if err != nil {
				slog.Error("a GitHub App cannot sign", "connection", record.ID, "error", err)
				continue
			}
			kept = append(kept, credentials.KeptMinter(record.URLPrefix, app, appLabel(record)))
		}
	}
	return kept
}
