package publishing

import (
	"errors"
	"slices"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

func TestEverySettingThatOpensARepositoryApplies(t *testing.T) {
	store, err := Open(t.TempDir(), File{})
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
	if len(effective.Groups) != 2 || !slices.Contains(effective.Groups[0].Protected, ".github/workflows/**") ||
		!slices.Equal(effective.Groups[1].Protected, []string{"deploy/**"}) {
		t.Errorf("groups = %+v", effective.Groups)
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
	store, err := Open(t.TempDir(), File{})
	if err != nil {
		t.Fatal(err)
	}
	web, _ := repo.Of("https://github.com/acme/web")
	if effective := store.For(web); effective.HasRules() || effective.BranchPrefix != "" || effective.PullRequests != nil {
		t.Errorf("effective = %+v", effective)
	}
}

func TestAPresetIsEditableAndRecommendedCanBeReplaced(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, File{})
	if err != nil {
		t.Fatal(err)
	}
	three := int32(3)
	if _, err := store.SetPreset(&gitdv1.Preset{Name: RecommendedName, ProtectedPaths: []string{"deploy/**"}, MaxChangedFiles: &three}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPreset(&gitdv1.Preset{Name: "strict", KeptPaths: []string{"migrations/**"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{Preset: RecommendedName}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{UrlPrefix: "https://github.com/acme", Preset: "strict"}); err != nil {
		t.Fatal(err)
	}
	web, _ := repo.Of("https://github.com/acme/web")
	effective := store.For(web)
	if effective.MaxFiles != 3 || !slices.Equal(effective.Groups[0].Protected, []string{"deploy/**"}) ||
		!slices.Equal(effective.Groups[1].Kept, []string{"migrations/**"}) {
		t.Errorf("effective = %+v", effective)
	}
	again, err := Open(directory, File{})
	if err != nil {
		t.Fatal(err)
	}
	if presets := again.Presets(); len(presets) != 2 || presets[0].GetOrigin() != gitdv1.Preset_ORIGIN_API {
		t.Errorf("presets after a restart = %v", presets)
	}
	if err := again.RemovePreset("strict"); !errors.Is(err, ErrInUse) {
		t.Errorf("removing a preset in use: %v", err)
	}
	if err := again.RemovePreset(RecommendedName); err != nil {
		t.Fatalf("removing the replacement of recommended: %v", err)
	}
	if presets := again.Presets(); presets[0].GetOrigin() != gitdv1.Preset_ORIGIN_BUILT_IN || presets[0].GetMaxChangedFiles() != 200 {
		t.Errorf("gitd's own recommended did not apply again: %v", presets)
	}
	if _, err := again.Set(&gitdv1.PublishSettings{UrlPrefix: "https://github.com/x", Preset: "missing"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("settings taking a missing preset: %v", err)
	}
}

func TestAllowedPathsFreeOnlyTheirOwnSettingsRules(t *testing.T) {
	store, err := Open(t.TempDir(), File{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{ProtectedPaths: []string{".gitmodules"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(&gitdv1.PublishSettings{UrlPrefix: "https://github.com/acme", Preset: RecommendedName,
		AllowedPaths: []string{".gitmodules"}}); err != nil {
		t.Fatal(err)
	}
	web, _ := repo.Of("https://github.com/acme/web")
	groups := store.For(web).Groups
	if len(groups) != 2 || len(groups[0].Allowed) != 0 || !slices.Equal(groups[1].Allowed, []string{".gitmodules"}) {
		t.Errorf("groups = %+v; the broader setting's .gitmodules must stay protected", groups)
	}
}
