package publishing

import (
	"slices"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

// Group is the rules one setting brings, its preset's and its own: paths a publish may not
// change, paths it may not delete, and the paths those rules leave free. From names the
// setting.
type Group struct {
	From      string
	Protected []string
	Kept      []string
	Allowed   []string
}

// Effective is what publishing a repository must follow, from every setting whose prefix
// opens it: each setting's rules as a group, checked on their own, so the paths one setting
// allows never free what another protects or keeps; the smallest file limit; the branch
// prefix and pull request choices of the most specific setting that has them. Applied names
// the settings it came from, broadest first.
type Effective struct {
	Groups       []Group
	MaxFiles     int32
	BranchPrefix string
	PullRequests *gitdv1.PullRequestSettings
	Applied      []string
}

// For is what publishing a repository must follow now.
func (store *Store) For(repository repo.Repo) Effective {
	applying := slices.DeleteFunc(store.All(), func(settings *gitdv1.PublishSettings) bool {
		return !opens(settings.GetUrlPrefix(), repository.HTTPS())
	})
	slices.SortStableFunc(applying, func(first, second *gitdv1.PublishSettings) int {
		return len(first.GetUrlPrefix()) - len(second.GetUrlPrefix())
	})
	var effective Effective
	for _, settings := range applying {
		preset := store.preset(settings.GetPreset())
		group := Group{
			From:      shown(settings.GetUrlPrefix()),
			Protected: appendNew(nil, preset.GetProtectedPaths(), settings.GetProtectedPaths()),
			Kept:      appendNew(nil, preset.GetKeptPaths(), settings.GetKeptPaths()),
			Allowed:   settings.GetAllowedPaths(),
		}
		if len(group.Protected) > 0 || len(group.Kept) > 0 {
			effective.Groups = append(effective.Groups, group)
		}
		limit := preset.GetMaxChangedFiles()
		if settings.MaxChangedFiles != nil {
			limit = settings.GetMaxChangedFiles()
		}
		if limit > 0 && (effective.MaxFiles == 0 || limit < effective.MaxFiles) {
			effective.MaxFiles = limit
		}
		if settings.GetBranchPrefix() != "" {
			effective.BranchPrefix = settings.GetBranchPrefix()
		}
		if settings.GetPullRequests() != nil {
			effective.PullRequests = settings.GetPullRequests()
		}
		effective.Applied = append(effective.Applied, shown(settings.GetUrlPrefix()))
	}
	return effective
}

// preset is the preset of a name as it applies now; none for no name.
func (store *Store) preset(name string) *gitdv1.Preset {
	if name == "" {
		return nil
	}
	store.guard.RLock()
	defer store.guard.RUnlock()
	_, preset := store.namedLocked(name)
	return preset
}

// HasRules says whether a publish has paths or a file limit to be checked against.
func (effective Effective) HasRules() bool {
	return len(effective.Groups) > 0 || effective.MaxFiles > 0
}

// appendNew adds to kept the paths of each list it does not have yet.
func appendNew(kept []string, lists ...[]string) []string {
	for _, list := range lists {
		for _, path := range list {
			if !slices.Contains(kept, path) {
				kept = append(kept, path)
			}
		}
	}
	return kept
}
