package publishing

import (
	"slices"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

func TestEverySettingThatOpensARepositoryApplies(t *testing.T) {
	store, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	fifty, ten := int32(50), int32(10)
	for _, settings := range []*gitdv1.PublishSettings{
		{Preset: "recommended", BranchPrefix: "agents/", MaxChangedFiles: &fifty,
			PullRequests: &gitdv1.PullRequestSettings{Open: gitdv1.PullRequestSettings_OPEN_WHEN_ASKED}},
		{UrlPrefix: "https://github.com/acme", ProtectedPaths: []string{"deploy/**"}, BranchPrefix: "agents/acme/",
			PullRequests: &gitdv1.PullRequestSettings{Open: gitdv1.PullRequestSettings_OPEN_ALWAYS, Draft: true}},
		{UrlPrefix: "https://github.com/acme/web", MaxChangedFiles: &ten},
		{UrlPrefix: "https://github.com/other", ProtectedPaths: []string{"never.txt"}},
	} {
		if _, err := store.Set(settings); err != nil {
			t.Fatal(err)
		}
	}
	web, _ := repo.Of("https://github.com/acme/web")
	effective := store.For(web)
	if !slices.Contains(effective.Protected, ".github/workflows/**") || !slices.Contains(effective.Protected, "deploy/**") ||
		slices.Contains(effective.Protected, "never.txt") {
		t.Errorf("protected = %v", effective.Protected)
	}
	if effective.MaxFiles != 10 || effective.BranchPrefix != "agents/acme/" ||
		effective.PullRequests.GetOpen() != gitdv1.PullRequestSettings_OPEN_ALWAYS || len(effective.Applied) != 3 {
		t.Errorf("effective = %+v", effective)
	}
	tools, _ := repo.Of("https://github.com/acme/tools")
	if store.For(tools).MaxFiles != 50 {
		t.Errorf("a narrower limit applied to a repository it does not open")
	}
}

func TestARepositoryNoSettingOpensHasNoRules(t *testing.T) {
	store, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	web, _ := repo.Of("https://github.com/acme/web")
	if effective := store.For(web); effective.HasRules() || effective.BranchPrefix != "" || effective.PullRequests != nil {
		t.Errorf("effective = %+v", effective)
	}
}
