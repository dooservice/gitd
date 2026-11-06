package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/dooservice/gitd/internal/config"
)

// gitHubTokenVariable is the variable a user's gitd reads the GitHub CLI's token from: its
// service sets it from `gh auth token` each time gitd starts.
const gitHubTokenVariable = "GH_TOKEN"

// userConfig is the configuration `gitd up` writes for a user who has none: gitd's places
// by their system's convention, the git found on their PATH, which a service's own PATH
// may not have, and, when GitHub is true, GitHub's repositories read with the GitHub CLI's
// token. It names where a secret is, never a secret.
type userConfig struct {
	places config.User
	git    string
	gitHub bool
}

// write writes the configuration where the user's goes, unless one is there already, and
// says whether it did. Only the user may read it, as later credentials may be added to it.
func (written userConfig) write() (bool, error) {
	text, err := written.text()
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(written.places.Config), 0o700); err != nil {
		return false, err
	}
	file, err := os.OpenFile(written.places.Config, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := file.Write(text); err != nil {
		_ = file.Close()
		return false, err
	}
	return true, file.Close()
}

// text is the configuration as TOML.
func (written userConfig) text() ([]byte, error) {
	var text bytes.Buffer
	text.WriteString("# gitd for you on this machine, written by `gitd up`. Every key is shown in\n" +
		"# https://github.com/dooservice/gitd/blob/main/packaging/gitd.toml\n\n")
	settings := struct {
		Socket string   `toml:"socket"`
		State  string   `toml:"state"`
		Roots  []string `toml:"roots"`
		Git    string   `toml:"git,omitempty"`
	}{written.places.Socket, written.places.State, []string{written.places.Root}, written.git}
	if err := toml.NewEncoder(&text).Encode(settings); err != nil {
		return nil, err
	}
	if written.gitHub {
		text.WriteString("\n# GitHub, with the GitHub CLI's token: gitd's service reads it (gh auth token)\n" +
			"# into $" + gitHubTokenVariable + " each time gitd starts, and it is never written down.\n" +
			"[[credentials]]\n" +
			"prefix = \"https://github.com\"\n" +
			"token_env = \"" + gitHubTokenVariable + "\"\n")
	}
	return text.Bytes(), nil
}
