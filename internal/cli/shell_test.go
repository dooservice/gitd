package cli

import "testing"

func TestAShellWordIsQuotedOnlyWhenItMustBe(t *testing.T) {
	for word, want := range map[string]string{
		"/home/ana/gitd.toml": "/home/ana/gitd.toml",
		"/home/my work":       "'/home/my work'",
		"it's":                `'it'\''s'`,
		"":                    "''",
	} {
		if got := shellWord(word); got != want {
			t.Errorf("shellWord(%q) = %s; want %s", word, got, want)
		}
	}
}
