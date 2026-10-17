package publishing

import (
	"slices"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

// Effective is what publishing a repository must follow, from every setting whose prefix
// opens it: their paths added up and the smallest file limit, so a narrower setting never
// loosens a broader one; the branch prefix and pull request choices of the most specific
// setting that has them. Applied names the settings it came from, broadest first.
type Effective struct {
	Rules
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
		preset := presetRules(settings.GetPreset())
		effective.Protected = appendNew(effective.Protected, preset.Protected, settings.GetProtectedPaths())
		effective.Kept = appendNew(effective.Kept, preset.Kept, settings.GetKeptPaths())
		limit := preset.MaxFiles
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

// HasRules says whether a publish has paths or a file limit to be checked against.
func (effective Effective) HasRules() bool {
	return len(effective.Protected) > 0 || len(effective.Kept) > 0 || effective.MaxFiles > 0
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
