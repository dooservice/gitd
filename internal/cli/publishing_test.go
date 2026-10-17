package cli

import (
	"bytes"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func TestSettingsReadAsTheirRules(t *testing.T) {
	var out bytes.Buffer
	fifty := int32(50)
	settingsLines(&out, &gitdv1.PublishSettings{
		UrlPrefix: "https://github.com/acme", Preset: gitdv1.Preset_PRESET_RECOMMENDED, ProtectedPaths: []string{"deploy/**"},
		MaxChangedFiles: &fifty, BranchPrefix: "agents/",
		PullRequests: &gitdv1.PullRequestSettings{Open: gitdv1.PullRequestSettings_OPEN_WHEN_ASKED, Draft: true,
			Labels: []string{"agent"}, RequestReviewFromRequester: true},
	})
	settingsLines(&out, &gitdv1.PublishSettings{BranchPrefix: "agents/", FromFile: true})
	want := "https://github.com/acme\n  preset: recommended\n  protected: deploy/**\n  at most 50 files\n  branches: agents/…\n" +
		"  pull requests: when asked, drafts, labelled agent, the requester asked to review\n" +
		"every repository  (gitd.toml)\n  branches: agents/…\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}
