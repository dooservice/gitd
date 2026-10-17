// Package pullrequests opens the pull request of a publish, on the host's API, with the
// same credential that pushed it, and writes its body: who asked for the work and what gitd
// checked, so a reviewer reads in a minute what they would otherwise dig for.
package pullrequests

import (
	"fmt"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/publishing"
)

// maxSubjects is at most how many commit subjects a body lists.
const maxSubjects = 10

// Account is what a body tells of a publish.
type Account struct {
	RequestedBy string
	Provenance  *gitdv1.Provenance
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
	if origin := provenance(account.Provenance); origin != "" {
		body.WriteString(origin + "\n\n")
	}
	body.WriteString("---\n**Checked by gitd**\n")
	body.WriteString(fmt.Sprintf("- Commits: %d — %s\n", len(account.Subjects), subjects(account.Subjects)))
	body.WriteString(fmt.Sprintf("- Changes: %s, +%d −%d\n", plural(account.Files, "file"), account.Added, account.Removed))
	if !account.Rules.HasRules() {
		body.WriteString("- Rules: none set\n")
	}
	for _, group := range account.Rules.Groups {
		body.WriteString("- Rules of " + group.From + ": passed (" + groupRules(group) + ")\n")
	}
	if limit := account.Rules.MaxFiles; limit > 0 {
		body.WriteString(fmt.Sprintf("- At most %d files: passed\n", limit))
	}
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

// provenance says where the work came from, as the publish said: the agent, its model,
// the session and the task.
func provenance(origin *gitdv1.Provenance) string {
	var parts []string
	if agent := origin.GetAgent(); agent != "" {
		if model := origin.GetModel(); model != "" {
			agent += " (" + model + ")"
		}
		parts = append(parts, "Agent: "+agent)
	} else if model := origin.GetModel(); model != "" {
		parts = append(parts, "Model: "+model)
	}
	if session := origin.GetSession(); session != "" {
		parts = append(parts, "session "+session)
	}
	if task := origin.GetTaskUrl(); task != "" {
		parts = append(parts, "task "+task)
	}
	return strings.Join(parts, " · ")
}

// groupRules says what one setting's rules asked of the publish.
func groupRules(group publishing.Group) string {
	var passed []string
	if len(group.Protected) > 0 {
		passed = append(passed, "unchanged: "+code(group.Protected))
	}
	if len(group.Kept) > 0 {
		passed = append(passed, "none deleted: "+code(group.Kept))
	}
	if len(group.Allowed) > 0 {
		passed = append(passed, "free: "+code(group.Allowed))
	}
	return strings.Join(passed, "; ")
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
