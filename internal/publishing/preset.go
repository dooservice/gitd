// Package publishing keeps the rules publishing follows for the repositories under each
// prefix: those of the configuration file, shown and never changed through the API, and
// those a platform sets through it, kept under the state directory and applied at once.
package publishing

import gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"

// Recommended is the preset most organisations want: the CI and who owns what untouched,
// no test deleted, and a publish small enough to review.
var Recommended = Rules{
	Protected: []string{".github/workflows/**", ".gitlab-ci.yml", "CODEOWNERS", ".gitmodules"},
	Kept:      []string{"**/*_test.*", "**/*.test.*", "**/test/**", "**/tests/**", "**/__tests__/**"},
	MaxFiles:  200,
}

// Rules is what a publish must pass: paths it may not change, paths it may not delete, and
// at most how many files it may change (0 for no limit).
type Rules struct {
	Protected []string
	Kept      []string
	MaxFiles  int32
}

// presetRules is what a preset brings.
func presetRules(preset gitdv1.Preset) Rules {
	if preset == gitdv1.Preset_PRESET_RECOMMENDED {
		return Recommended
	}
	return Rules{}
}
