package git

import "testing"

func TestGitHubExplainsItselfInARemoteLine(t *testing.T) {
	lines := []string{
		"remote: Repository not found.",
		"fatal: repository 'https://github.com/acme/nope/' not found",
	}
	if got := lastWords(lines); got != "remote: Repository not found." {
		t.Errorf("lastWords = %q", got)
	}
	if got := classify(lines); got != NotFound {
		t.Errorf("classify = %v", got)
	}
}

func TestAskingForACredentialReadsAsAuthRequired(t *testing.T) {
	for _, said := range []string{
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled",
		"git@github.com: Permission denied (publickey).",
		"fatal: Authentication failed for 'https://github.com/acme/private/'",
	} {
		if got := classify([]string{said}); got != AuthRequired {
			t.Errorf("classify(%q) = %v", said, got)
		}
	}
}

func TestARefusedCredentialIsToldFromNoneAtAll(t *testing.T) {
	if got := (&Failure{Reason: AuthRequired}).AccessRefused(true).Reason; got != AuthRefused {
		t.Errorf("offered: %v", got)
	}
	if got := (&Failure{Reason: AuthRequired}).AccessRefused(false).Reason; got != AuthRequired {
		t.Errorf("not offered: %v", got)
	}
}

func TestAHintIsNeverTheLastWord(t *testing.T) {
	lines := []string{"error: pathspec 'x' did not match", "hint: try again"}
	if got := lastWords(lines); got != lines[0] {
		t.Errorf("lastWords = %q", got)
	}
}

func TestEveryForgeSaysItsRefusalsItsOwnWay(t *testing.T) {
	for said, want := range map[string]Reason{
		"remote: The project you were looking for could not be found or you don't have permission to view it.":             NotFound,
		"remote: HTTP Basic: Access denied. If a password was provided for Git authentication, the password was incorrect": AuthRequired,
		"conq: repository does not exist.":                                    NotFound,
		"Repository access denied. Access via a deployment key is read-only.": AuthRequired,
		"remote: Unauthorized":                                                AuthRequired,
		"Gitea: Unauthorized":                                                 AuthRequired,
	} {
		if got := classify([]string{said}); got != want {
			t.Errorf("classify(%q) = %v; want %v", said, got, want)
		}
	}
}
