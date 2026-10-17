package publishing

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
)

// openings are how the file names the ways of opening pull requests.
var (
	openings = map[string]gitdv1.PullRequestSettings_Open{
		"":           gitdv1.PullRequestSettings_OPEN_UNSPECIFIED,
		"when_asked": gitdv1.PullRequestSettings_OPEN_WHEN_ASKED,
		"always":     gitdv1.PullRequestSettings_OPEN_ALWAYS,
	}
)

// File is the presets and settings of the configuration file.
type File struct {
	Presets  []*gitdv1.Preset
	Settings []*gitdv1.PublishSettings
}

// FromFile reads the presets and settings of the configuration file, checked as the API
// checks its own. `[publish] branch_prefix` is the branch prefix of every repository.
func FromFile(publish config.Publish) (File, error) {
	presets, err := presetsFromFile(publish.Presets)
	if err != nil {
		return File{}, err
	}
	settings, err := settingsFromFile(publish, presets)
	if err != nil {
		return File{}, err
	}
	return File{Presets: presets, Settings: settings}, nil
}

func presetsFromFile(declared []config.PublishPreset) ([]*gitdv1.Preset, error) {
	read := make([]*gitdv1.Preset, 0, len(declared))
	for index, preset := range declared {
		kept, err := normalizedPreset(&gitdv1.Preset{
			Name: preset.Name, ProtectedPaths: preset.ProtectedPaths, KeptPaths: preset.KeptPaths,
			MaxChangedFiles: preset.MaxChangedFiles,
		}, gitdv1.Preset_ORIGIN_FILE)
		if err != nil {
			return nil, fmt.Errorf("publish.presets[%d]: %w", index, err)
		}
		if slices.ContainsFunc(read, func(other *gitdv1.Preset) bool { return other.GetName() == kept.GetName() }) {
			return nil, fmt.Errorf("publish.presets: %q is written twice", kept.GetName())
		}
		read = append(read, kept)
	}
	return read, nil
}

func settingsFromFile(publish config.Publish, presets []*gitdv1.Preset) ([]*gitdv1.PublishSettings, error) {
	read := make([]*gitdv1.PublishSettings, 0, len(publish.Settings)+1)
	everyRepository := -1
	for index, setting := range publish.Settings {
		converted, err := fromSetting(setting)
		if err != nil {
			return nil, fmt.Errorf("publish.settings[%d] (%q): %w", index, setting.Prefix, err)
		}
		known := slices.ContainsFunc(presets, func(preset *gitdv1.Preset) bool { return preset.GetName() == setting.Preset })
		if setting.Preset != "" && setting.Preset != RecommendedName && !known {
			return nil, fmt.Errorf("publish.settings[%d] (%q): %w: no preset %q", index, setting.Prefix, ErrInvalid, setting.Preset)
		}
		for _, earlier := range read {
			if earlier.GetUrlPrefix() == converted.GetUrlPrefix() {
				return nil, fmt.Errorf("publish.settings: %q is written twice", shown(converted.GetUrlPrefix()))
			}
		}
		if converted.GetUrlPrefix() == "" {
			everyRepository = len(read)
		}
		read = append(read, converted)
	}
	if prefix := publish.BranchPrefix; prefix != "" {
		switch {
		case everyRepository < 0:
			read = append(read, &gitdv1.PublishSettings{BranchPrefix: prefix, FromFile: true})
		case read[everyRepository].GetBranchPrefix() == "":
			read[everyRepository].BranchPrefix = prefix
		case read[everyRepository].GetBranchPrefix() != prefix:
			return nil, fmt.Errorf("publish: branch_prefix %q and the branch prefix of the settings of every repository, %q, differ",
				prefix, read[everyRepository].GetBranchPrefix())
		}
	}
	if err := consistent(read); err != nil {
		return nil, fmt.Errorf("publish.settings: %w", err)
	}
	return read, nil
}

func fromSetting(setting config.PublishSetting) (*gitdv1.PublishSettings, error) {
	converted := &gitdv1.PublishSettings{
		UrlPrefix:       setting.Prefix,
		Preset:          setting.Preset,
		ProtectedPaths:  setting.ProtectedPaths,
		KeptPaths:       setting.KeptPaths,
		AllowedPaths:    setting.AllowedPaths,
		MaxChangedFiles: setting.MaxChangedFiles,
		BranchPrefix:    setting.BranchPrefix,
		FromFile:        true,
	}
	if pullRequests := setting.PullRequests; pullRequests.Open != "" || pullRequests.Draft ||
		len(pullRequests.Labels) > 0 || pullRequests.RequestReviewFromRequester {
		open, known := openings[pullRequests.Open]
		if !known {
			return nil, fmt.Errorf("%w: pull_requests.open %q is neither \"when_asked\" nor \"always\"", ErrInvalid, pullRequests.Open)
		}
		converted.PullRequests = &gitdv1.PullRequestSettings{
			Open: open, Draft: pullRequests.Draft, Labels: pullRequests.Labels,
			RequestReviewFromRequester: pullRequests.RequestReviewFromRequester,
		}
	}
	return normalized(converted)
}

// cloned is a copy of settings that whoever holds the original cannot change.
func cloned(settings *gitdv1.PublishSettings) *gitdv1.PublishSettings {
	return proto.Clone(settings).(*gitdv1.PublishSettings)
}
