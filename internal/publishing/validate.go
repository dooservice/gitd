package publishing

import (
	"errors"
	"fmt"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/credentials"
)

// ErrInvalid is settings that cannot be kept, saying why.
var ErrInvalid = errors.New("invalid publishing settings")

// Limits on what settings may hold, so that no setting makes a publish slow to check.
const (
	maxPaths      = 200
	maxPathLength = 512
	maxLabels     = 20
	maxLabelSize  = 50
)

// normalized checks settings and returns them with their prefix written as a repository's
// address starts.
func normalized(settings *gitdv1.PublishSettings) (*gitdv1.PublishSettings, error) {
	prefix := ""
	if settings.GetUrlPrefix() != "" {
		prefix = credentials.CanonicalPrefix(settings.GetUrlPrefix())
		if !(credentials.Credential{Prefix: prefix}).NamesHost() {
			return nil, invalid("the prefix %q must name a host, e.g. https://github.com/acme", settings.GetUrlPrefix())
		}
	}
	if preset := settings.GetPreset(); preset != "" && !presetName.MatchString(preset) {
		return nil, invalid("%q is not a preset's name", preset)
	}
	for name, paths := range map[string][]string{
		"protected": settings.GetProtectedPaths(), "kept": settings.GetKeptPaths(), "allowed": settings.GetAllowedPaths(),
	} {
		if err := validPaths(name, paths); err != nil {
			return nil, err
		}
	}
	if settings.MaxChangedFiles != nil && settings.GetMaxChangedFiles() < 1 {
		return nil, invalid("max_changed_files must be at least 1, or unset")
	}
	if !config.ValidBranchPrefix(settings.GetBranchPrefix()) {
		return nil, invalid("the branch prefix %q cannot start a branch name", settings.GetBranchPrefix())
	}
	if err := validPullRequests(settings.GetPullRequests()); err != nil {
		return nil, err
	}
	kept := cloned(settings)
	kept.UrlPrefix = prefix
	return kept, nil
}

// validPaths checks paths as git's glob pathspecs: no magic of their own, nothing that is
// not one line.
func validPaths(name string, paths []string) error {
	if len(paths) > maxPaths {
		return invalid("at most %d %s paths", maxPaths, name)
	}
	for _, path := range paths {
		switch {
		case path == "" || len(path) > maxPathLength:
			return invalid("a %s path must have 1 to %d characters", name, maxPathLength)
		case strings.HasPrefix(path, ":"):
			return invalid("the %s path %q starts with ':', which git reads as magic", name, path)
		case strings.ContainsAny(path, "\x00\n\r"):
			return invalid("the %s path %q is not one line", name, path)
		}
	}
	return nil
}

func validPullRequests(pullRequests *gitdv1.PullRequestSettings) error {
	if pullRequests == nil {
		return nil
	}
	if _, known := gitdv1.PullRequestSettings_Open_name[int32(pullRequests.GetOpen())]; !known {
		return invalid("no way of opening pull requests %d", pullRequests.GetOpen())
	}
	if len(pullRequests.GetLabels()) > maxLabels {
		return invalid("at most %d labels", maxLabels)
	}
	for _, label := range pullRequests.GetLabels() {
		if label == "" || len(label) > maxLabelSize || strings.ContainsAny(label, "\x00\n\r") {
			return invalid("a label must be one line of 1 to %d characters", maxLabelSize)
		}
	}
	return nil
}

// consistent checks that no narrower setting loosens a broader one's branch prefix: where
// both set one, the narrower's starts with the broader's.
func consistent(all []*gitdv1.PublishSettings) error {
	for _, broader := range all {
		for _, narrower := range all {
			if broader == narrower || !opens(broader.GetUrlPrefix(), narrower.GetUrlPrefix()) {
				continue
			}
			wide, narrow := broader.GetBranchPrefix(), narrower.GetBranchPrefix()
			if wide != "" && narrow != "" && !strings.HasPrefix(narrow, wide) {
				return invalid("the branch prefix %q of %q does not start with %q, the one of %q, which it falls under",
					narrow, shown(narrower.GetUrlPrefix()), wide, shown(broader.GetUrlPrefix()))
			}
		}
	}
	return nil
}

// opens says whether the settings of prefix apply to what starts with other.
func opens(prefix, other string) bool {
	return prefix == "" || other == prefix || strings.HasPrefix(other, prefix+"/")
}

// shown is how a prefix is named in a message: empty is every repository.
func shown(prefix string) string {
	if prefix == "" {
		return "every repository"
	}
	return prefix
}

func invalid(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, values...))
}
