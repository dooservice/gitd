// Package pullrequests opens the pull request of a publish, on the host's API, with the
// same credential that pushed it, and writes its body: who asked for the work and what gitd
// checked, so a reviewer reads in a minute what they would otherwise dig for.
package pullrequests

import (
	"fmt"
	"strings"

	"github.com/dooservice/gitd/internal/publishing"
)

// maxSubjects is at most how many commit subjects a body lists.
const maxSubjects = 10

// Account is what a body tells of a publish.
type Account struct {
	RequestedBy string
	Subjects    []string
	Files       int
	Added       int
	Removed     int
	Rules       publishing.Effective
	Submodules  []string
}

// Body is a pull request's body: the platform's text first, when given, then gitd's
// account of the publish.
func Body(platform string, account Account) string {
	var body strings.Builder
	if text := strings.TrimSpace(platform); text != "" {
		body.WriteString(text + "\n\n")
	}
	if account.RequestedBy != "" {
		body.WriteString("Requested by @" + account.RequestedBy + "\n\n")
	}
	body.WriteString("---\n**Checked by gitd**\n")
	body.WriteString(fmt.Sprintf("- Commits: %d — %s\n", len(account.Subjects), subjects(account.Subjects)))
	body.WriteString(fmt.Sprintf("- Changes: %s, +%d −%d\n", plural(account.Files, "file"), account.Added, account.Removed))
	body.WriteString("- Rules: " + rules(account.Rules) + "\n")
	if len(account.Submodules) > 0 {
		body.WriteString("- Submodules published: " + strings.Join(account.Submodules, ", ") + "\n")
	}
	return body.String()
}

func subjects(listed []string) string {
	if len(listed) <= maxSubjects {
		return strings.Join(listed, " · ")
	}
	return fmt.Sprintf("%s · and %d more", strings.Join(listed[:maxSubjects], " · "), len(listed)-maxSubjects)
}

// rules says what the publish passed.
func rules(effective publishing.Effective) string {
	if !effective.HasRules() {
		return "none set"
	}
	var passed []string
	if len(effective.Protected) > 0 {
		passed = append(passed, "unchanged: "+code(effective.Protected))
	}
	if len(effective.Kept) > 0 {
		passed = append(passed, "none deleted: "+code(effective.Kept))
	}
	if effective.MaxFiles > 0 {
		passed = append(passed, fmt.Sprintf("at most %d files", effective.MaxFiles))
	}
	return "passed (" + strings.Join(passed, "; ") + ") from the settings of " + strings.Join(effective.Applied, " and ")
}

func code(paths []string) string {
	quoted := make([]string, len(paths))
	for index, path := range paths {
		quoted[index] = "`" + path + "`"
	}
	return strings.Join(quoted, ", ")
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
