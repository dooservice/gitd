package pullrequests

import (
	"strings"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/publishing"
)

func TestABodySaysWhoAskedAndWhatGitdChecked(t *testing.T) {
	body := Body("Fixes the login redirect.", Account{
		RequestedBy: "josue",
		Subjects:    []string{"fix login redirect", "add test for expired session"},
		Files:       4, Added: 82, Removed: 17,
		Rules: publishing.Effective{MaxFiles: 200, Groups: []publishing.Group{{
			From: "https://github.com/acme", Protected: []string{".github/workflows/**"}, Allowed: []string{".gitmodules"},
		}}},
		Submodules: []string{"libs/auth"},
	})
	for _, want := range []string{
		"Fixes the login redirect.\n\nRequested by @josue",
		"**Checked by gitd**",
		"- Commits: 2 — fix login redirect · add test for expired session",
		"- Changes: 4 files, +82 −17",
		"- Rules of https://github.com/acme: passed (unchanged: `.github/workflows/**`; free: `.gitmodules`)",
		"- At most 200 files: passed",
		"- Submodules published: libs/auth",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body lacks %q:\n%s", want, body)
		}
	}
}

func TestABodyWithoutRulesOrRequesterSaysSo(t *testing.T) {
	body := Body("", Account{Subjects: []string{"one"}, Files: 1})
	if strings.Contains(body, "Requested by") || !strings.Contains(body, "- Rules: none set") ||
		!strings.Contains(body, "1 file,") || strings.HasPrefix(body, "\n") {
		t.Errorf("body:\n%s", body)
	}
}

func TestABodySaysWhereTheWorkCameFrom(t *testing.T) {
	body := Body("", Account{Subjects: []string{"one"}, Files: 1, Provenance: &gitdv1.Provenance{
		Agent: "openhands", Model: "claude-opus-5-5", Session: "run-42", TaskUrl: "https://github.com/acme/web/issues/7",
	}})
	want := "Agent: openhands (claude-opus-5-5) · session run-42 · task https://github.com/acme/web/issues/7\n"
	if !strings.Contains(body, want) {
		t.Errorf("the body lacks %q:\n%s", want, body)
	}
}
