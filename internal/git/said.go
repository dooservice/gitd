package git

import (
	"slices"
	"strings"
)

// lastWords is the line of git's stderr that says what went wrong.
//
// GitHub explains itself in `remote:` lines and git then adds a `fatal:` that repeats the
// address, so a `remote:` line wins when there is one.
func lastWords(lines []string) string {
	for _, line := range slices.Backward(lines) {
		if said, ok := strings.CutPrefix(line, "remote: "); ok && strings.TrimSpace(said) != "" {
			return line
		}
	}
	for _, line := range slices.Backward(lines) {
		if !strings.HasPrefix(line, "hint:") {
			return line
		}
	}
	return ""
}

// classify reads what git said into a reason. A credential refused reads the same as no
// credential at all; whoever offered one tells them apart.
func classify(lines []string) Reason {
	said := strings.ToLower(strings.Join(lines, "\n"))
	switch {
	case mentions(said,
		"could not read username",
		"terminal prompts disabled",
		"authentication failed",
		"permission denied (publickey",
		"invalid username or password",
		"requested url returned error: 401",
		"requested url returned error: 403",
	):
		return AuthRequired
	case mentions(said,
		"not found",
		"couldn't find remote ref",
		"did not match any",
		"unknown revision",
		"not a valid object name",
		"bad object",
		"needed a single revision",
		"does not appear to be a git repository",
	):
		return NotFound
	}
	return Failed
}

func mentions(said string, phrases ...string) bool {
	return slices.ContainsFunc(phrases, func(phrase string) bool {
		return strings.Contains(said, phrase)
	})
}
