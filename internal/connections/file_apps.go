package connections

import (
	"fmt"
	"os"
	"strings"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/githubapp"
)

// FileApps are the GitHub Apps the configuration file declares, as credentials: each
// App's key read from its file each time it signs, so replacing the file rotates the key.
// A key that cannot be read now stops the start.
type FileApps struct {
	kept []credentials.Credential
}

// NewFileApps reads the Apps among declared, reaching GitHub as github says.
func NewFileApps(declared []config.Credential, github GitHub) (*FileApps, error) {
	apps := &FileApps{}
	for index, credential := range declared {
		setting := credential.GitHubApp
		if setting == nil {
			continue
		}
		prefix := credentials.CanonicalPrefix(credential.Prefix)
		if !(credentials.Credential{Prefix: prefix}).NamesHost() {
			return nil, fmt.Errorf("credentials[%d]: a github_app's prefix names its host, e.g. https://github.com/acme", index)
		}
		keyFile := setting.PrivateKeyFile
		signer, err := githubapp.NewSigner(setting.AppID, func() ([]byte, error) { return os.ReadFile(keyFile) })
		if err != nil {
			return nil, fmt.Errorf("credentials[%d] (%q): %w", index, credential.Prefix, err)
		}
		host, _, _ := strings.Cut(strings.TrimPrefix(prefix, "https://"), "/")
		app := githubapp.NewApp(signer, github.API(host))
		label := appLabel(Record{AppID: setting.AppID})
		apps.kept = append(apps.kept, credentials.KeptMinter(prefix, app, label))
	}
	return apps, nil
}

// Credentials are the Apps' credentials, which make a token for each operation.
func (apps *FileApps) Credentials() []credentials.Credential {
	return apps.kept
}
