package git

import (
	"slices"
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
