package connections

import (
	"strconv"

	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/credentials"
)

// FilePrefix starts the IDs of the connections the configuration file declares.
const FilePrefix = "file-"

// kinds are the contract's names for each kind.
var kinds = map[Kind]gitdv1.Connection_Kind{
	Token:       gitdv1.Connection_KIND_TOKEN,
	GitHubApp:   gitdv1.Connection_KIND_GITHUB_APP,
	GitHubLogin: gitdv1.Connection_KIND_GITHUB_LOGIN,
}

// Shown is a connection as the API shows it: everything but its secret.
func (record Record) Shown() *gitdv1.Connection {
	shown := &gitdv1.Connection{
		Id: record.ID, Kind: kinds[record.Kind], UrlPrefix: record.URLPrefix, ActsAs: record.ActsAs,
		AppId: record.AppID, ManageUrl: record.ManageURL, Created: timestamppb.New(record.Created),
	}
	if !record.Expires.IsZero() {
		shown.Expires = timestamppb.New(record.Expires)
	}
	return shown
}

// ShownFromFile are the connections the configuration file declares, as the API shows
// them: named `file-1`, `file-2`… in the file's order, never their secrets.
func ShownFromFile(declared []config.Credential) []*gitdv1.Connection {
	shown := make([]*gitdv1.Connection, 0, len(declared))
	for index, credential := range declared {
		kind := gitdv1.Connection_KIND_TOKEN
		if credential.SSHKey != "" {
			kind = gitdv1.Connection_KIND_SSH_KEY
		}
		shown = append(shown, &gitdv1.Connection{
			Id:        FilePrefix + strconv.Itoa(index+1),
			Kind:      kind,
			UrlPrefix: credentials.CanonicalPrefix(credential.Prefix),
			FromFile:  true,
		})
	}
	return shown
}
