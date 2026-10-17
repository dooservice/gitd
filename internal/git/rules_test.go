package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGitFollowsNoRedirectAndRunsNoHookOrHelper(t *testing.T) {
	for _, local := range []bool{false, true} {
		flags := hardened(local)
		for _, want := range []string{
			"http.followRedirects=false", "credential.helper=", "core.hooksPath=/dev/null", "core.fsmonitor=false",
		} {
			if !slices.Contains(flags, want) {
				t.Errorf("hardened(%v) lacks %s", local, want)
			}
		}
	}
}

func TestADiffThatFindsDifferencesAnswersRatherThanFails(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(first, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner("git")
	out, err := runner.Run(context.Background(), Cmd{Dir: dir, Differences: true,
		Args: []string{"diff", "--no-index", "--numstat", "--", "a", "b"}})
	if err != nil || !strings.HasPrefix(out, "1\t1\t") {
		t.Errorf("Run = %q, %v", out, err)
	}
	if _, err := runner.Run(context.Background(), Cmd{Dir: dir,
		Args: []string{"diff", "--no-index", "--", "a", "b"}}); err == nil {
		t.Error("without Differences, status 1 is a failure")
	}
}
