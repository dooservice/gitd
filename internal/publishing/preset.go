// Package publishing keeps the rules publishing follows for the repositories under each
// prefix, and the presets — named sets of rules — those settings take: gitd's own, those of
// the configuration file, shown and never changed through the API, and those a platform
// sets through it, kept under the state directory and applied at once.
package publishing

import (
	"fmt"
	"regexp"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// RecommendedName is the name of gitd's own preset.
const RecommendedName = "recommended"

// recommendedFiles is at most how many files the recommended preset lets a publish change.
const recommendedFiles = 200

// presetName is what a preset may be called.
var presetName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// builtIn is gitd's own preset, which most organisations want: the CI and who owns what
// untouched, no test deleted, and a publish small enough to review. A preset of its name
// set through the file or the API replaces it.
func builtIn() *gitdv1.Preset {
	files := int32(recommendedFiles)
	return &gitdv1.Preset{
		Name:            RecommendedName,
		ProtectedPaths:  []string{".github/workflows/**", ".gitlab-ci.yml", "CODEOWNERS", ".gitmodules"},
		KeptPaths:       []string{"**/*_test.*", "**/*.test.*", "**/test/**", "**/tests/**", "**/__tests__/**"},
		MaxChangedFiles: &files,
		Origin:          gitdv1.Preset_ORIGIN_BUILT_IN,
	}
}

// normalizedPreset checks a preset and returns it as kept, from origin.
func normalizedPreset(preset *gitdv1.Preset, origin gitdv1.Preset_Origin) (*gitdv1.Preset, error) {
	if !presetName.MatchString(preset.GetName()) {
		return nil, invalid("a preset's name is 1 to 64 lowercase letters, digits, '.', '_' or '-', not %q", preset.GetName())
	}
	for name, paths := range map[string][]string{"protected": preset.GetProtectedPaths(), "kept": preset.GetKeptPaths()} {
		if err := validPaths(name, paths); err != nil {
			return nil, fmt.Errorf("preset %q: %w", preset.GetName(), err)
		}
	}
	if preset.MaxChangedFiles != nil && preset.GetMaxChangedFiles() < 1 {
		return nil, invalid("preset %q: max_changed_files must be at least 1, or unset", preset.GetName())
	}
	kept := clonedPreset(preset)
	kept.Origin = origin
	return kept, nil
}
