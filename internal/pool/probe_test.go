package pool

import "testing"

func TestAListingSaysEachBranchAndTheCommitItPointsAt(t *testing.T) {
	out := "ref: refs/heads/18.0\tHEAD\n" +
		"aaaa1111\tHEAD\n" +
		"bbbb2222\trefs/heads/17.0\n" +
		"aaaa1111\trefs/heads/18.0\n"

	answer := listed(out)

	if answer.GetDefaultBranch() != "18.0" {
		t.Errorf("default = %q", answer.GetDefaultBranch())
	}
	if got := answer.GetBranches(); len(got) != 2 || got[0] != "17.0" || got[1] != "18.0" {
		t.Errorf("branches = %v", got)
	}
	if answer.GetCommits()["17.0"] != "bbbb2222" || answer.GetCommits()["18.0"] != "aaaa1111" {
		t.Errorf("commits = %v", answer.GetCommits())
	}
}
