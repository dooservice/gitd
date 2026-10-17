package connections

import (
	"testing"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
)

func TestAConnectionIsShownWithoutItsSecret(t *testing.T) {
	shown := Record{ID: "app-1", Kind: GitHubApp, URLPrefix: "https://github.com/acme", ActsAs: "acme-agents[bot]",
		AppID: 7, Created: time.Now(), Secret: "-----BEGIN PRIVATE KEY-----"}.Shown()
	if shown.GetKind() != gitdv1.Connection_KIND_GITHUB_APP || shown.GetActsAs() != "acme-agents[bot]" ||
		shown.GetAppId() != 7 || shown.GetExpires() != nil {
		t.Errorf("shown = %v", shown)
	}
}

func TestTheFilesConnectionsAreShownByTheirOrder(t *testing.T) {
	shown := ShownFromFile([]config.Credential{
		{Prefix: "https://GitHub.com/acme/", TokenFile: "/run/token"},
		{Prefix: "git@gitlab.com:acme/", SSHKey: "/etc/gitd/key"},
	})
	if len(shown) != 2 || shown[0].GetId() != "file-1" || shown[0].GetUrlPrefix() != "https://github.com/acme" ||
		shown[1].GetKind() != gitdv1.Connection_KIND_SSH_KEY || !shown[1].GetFromFile() {
		t.Errorf("shown = %v", shown)
	}
}
