package publishing

import (
	"errors"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
)

func TestSettingsSetThroughTheAPIAreKeptAcrossRestarts(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "publishing")
	store, err := Open(directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	set, err := store.Set(&gitdv1.PublishSettings{
		UrlPrefix: "https://GitHub.com/acme/", Preset: "recommended", BranchPrefix: "agents/",
		PullRequests: &gitdv1.PullRequestSettings{Open: gitdv1.PullRequestSettings_OPEN_ALWAYS, Labels: []string{"agent"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if set.GetUrlPrefix() != "https://github.com/acme" {
		t.Errorf("the prefix kept is %q", set.GetUrlPrefix())
	}
	again, err := Open(directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if all := again.All(); len(all) != 1 || !proto.Equal(all[0], set) {
		t.Errorf("after a restart: %v", all)
	}
}

func TestSettingsReplaceThoseOfTheirPrefixAndAreRemoved(t *testing.T) {
	store, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, branchPrefix := range []string{"agents/", "bots/"} {
		if _, err := store.Set(&gitdv1.PublishSettings{UrlPrefix: "https://github.com/acme", BranchPrefix: branchPrefix}); err != nil {
			t.Fatal(err)
		}
	}
	if all := store.All(); len(all) != 1 || all[0].GetBranchPrefix() != "bots/" {
		t.Errorf("all = %v", all)
	}
	if err := store.Remove("https://github.com/acme/"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(store.Remove("https://github.com/acme"), ErrNotFound) {
		t.Error("removed settings are still there")
	}
}

func TestTheFilesSettingsAreShownAndNeverChangedThroughTheAPI(t *testing.T) {
	fromFile, err := FromFile(config.Publish{BranchPrefix: "agents/", Settings: []config.PublishSetting{
		{Prefix: "https://github.com/acme", Preset: "recommended", PullRequests: config.PullRequests{Open: "always"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.TempDir(), fromFile)
	if err != nil {
		t.Fatal(err)
	}
	all := store.All()
	if len(all) != 2 || !all[0].GetFromFile() || all[1].GetBranchPrefix() != "agents/" {
		t.Fatalf("all = %v", all)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{UrlPrefix: "https://github.com/acme"}); !errors.Is(err, ErrFromFile) {
		t.Errorf("setting the file's prefix: %v", err)
	}
	if err := store.Remove(""); !errors.Is(err, ErrFromFile) {
		t.Errorf("removing the file's prefix: %v", err)
	}
}

func TestANarrowerBranchPrefixMayNotLoosenABroaderOne(t *testing.T) {
	store, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{BranchPrefix: "agents/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{UrlPrefix: "https://github.com/acme", BranchPrefix: "agents/acme/"}); err != nil {
		t.Errorf("a narrower prefix under the broader one was refused: %v", err)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{UrlPrefix: "https://github.com/other", BranchPrefix: "bots/"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a narrower prefix outside the broader one: %v", err)
	}
}

func TestSettingsThatCannotBeKeptAreRefusedSayingWhy(t *testing.T) {
	store, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	for name, settings := range map[string]*gitdv1.PublishSettings{
		"no host":       {UrlPrefix: "acme"},
		"magic path":    {ProtectedPaths: []string{":(top)ci.yml"}},
		"empty path":    {KeptPaths: []string{""}},
		"two lines":     {ProtectedPaths: []string{"a\nb"}},
		"no file limit": {MaxChangedFiles: &zero},
		"bad branch":    {BranchPrefix: "-x/"},
		"unknown open":  {PullRequests: &gitdv1.PullRequestSettings{Open: 9}},
		"empty label":   {PullRequests: &gitdv1.PullRequestSettings{Labels: []string{""}}},
	} {
		if _, err := store.Set(settings); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(store.All()) != 0 {
		t.Errorf("something refused was kept: %v", store.All())
	}
}

func TestTheFileIsCheckedAsTheAPIIs(t *testing.T) {
	for name, publish := range map[string]config.Publish{
		"unknown preset": {Settings: []config.PublishSetting{{Preset: "strict"}}},
		"unknown open":   {Settings: []config.PublishSetting{{PullRequests: config.PullRequests{Open: "sometimes"}}}},
		"twice":          {Settings: []config.PublishSetting{{Prefix: "https://github.com/acme"}, {Prefix: "https://github.com/acme/"}}},
		"two prefixes":   {BranchPrefix: "agents/", Settings: []config.PublishSetting{{BranchPrefix: "bots/"}}},
		"loosened":       {BranchPrefix: "agents/", Settings: []config.PublishSetting{{Prefix: "https://github.com/acme", BranchPrefix: "bots/"}}},
	} {
		if _, err := FromFile(publish); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
