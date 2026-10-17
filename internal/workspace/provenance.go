package workspace

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// provenanceLength is the most characters a field of a provenance may hold.
const provenanceLength = 200

// validProvenance refuses a provenance whose fields are not short single lines, or whose
// task address is not an https one, before anything is done.
func validProvenance(provenance *gitdv1.Provenance) error {
	if provenance == nil {
		return nil
	}
	for name, value := range map[string]string{
		"agent": provenance.GetAgent(), "model": provenance.GetModel(), "session": provenance.GetSession(),
		"task_url": provenance.GetTaskUrl(),
	} {
		if utf8.RuneCountInString(value) > provenanceLength || strings.ContainsFunc(value, unicode.IsControl) {
			return git.Fail(git.BadAddress, "provenance %s: one line of at most %d characters", name, provenanceLength)
		}
	}
	if task := provenance.GetTaskUrl(); task != "" {
		parsed, err := url.Parse(task)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return git.Fail(git.BadAddress, "provenance task_url: %q is not an https address", task)
		}
	}
	return nil
}
