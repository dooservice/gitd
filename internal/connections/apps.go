package connections

import (
	"strconv"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/githubapp"
)

// app is the App of a connection: its private key read from the store each time it signs,
// so the key replaced under the connection is used from then on.
func (store *Store) app(record Record) (credentials.Minter, error) {
	return store.minter(record.ID, func() (credentials.Minter, error) {
		signer, err := githubapp.NewSigner(record.AppID, func() ([]byte, error) {
			_, key, err := store.Get(record.ID)
			return key, err
		})
		if err != nil {
			return nil, err
		}
		return githubapp.NewApp(signer, store.github.api(record.Host)), nil
	})
}

// appLabel names an App in the logs.
func appLabel(record Record) string {
	return "GitHub App " + strconv.FormatInt(record.AppID, 10)
}
