package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=protocol.file.allow", "GIT_CONFIG_VALUE_0=always")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestABranchThatMovesASubmoduleIsFetchedBeforeTheSubmodulesPoolHasItsCommit(t *testing.T) {
	root := t.TempDir()
	sub, super := filepath.Join(root, "sub"), filepath.Join(root, "super")
	subPool, superPool := filepath.Join(root, "sub.git"), filepath.Join(root, "super.git")
	work := filepath.Join(root, "work")
	run(t, root, "init", "-q", "-b", "main", sub)
	run(t, sub, "commit", "-q", "--allow-empty", "-m", "a")
	run(t, root, "init", "-q", "-b", "main", super)
	run(t, super, "submodule", "-q", "add", sub, "sub")
	run(t, super, "commit", "-q", "-m", "submodule")
	run(t, root, "clone", "-q", "--bare", sub, subPool)
	run(t, root, "clone", "-q", "--bare", super, superPool)
	run(t, root, "clone", "-q", superPool, work)
	if err := os.MkdirAll(filepath.Join(work, ".git", "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, work, "init", "-q", "--separate-git-dir", filepath.Join(work, ".git", "modules", "sub"), filepath.Join(root, "mounted"))
	if err := os.RemoveAll(filepath.Join(work, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "mounted"), filepath.Join(work, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "sub", ".git"), []byte("gitdir: ../.git/modules/sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, filepath.Join(work, "sub"), "config", "core.worktree", "../../../sub")
	run(t, filepath.Join(work, "sub"), "remote", "add", "origin", "file://"+subPool)
	run(t, filepath.Join(work, "sub"), "fetch", "-q", "origin")
	run(t, work, "config", "submodule.sub.url", "file://"+subPool)
	run(t, work, "config", "submodule.sub.active", "true")

	run(t, sub, "commit", "-q", "--allow-empty", "-m", "b")
	run(t, filepath.Join(super, "sub"), "pull", "-q", "origin", "main")
	run(t, super, "commit", "-q", "-am", "the submodule moves")
	run(t, superPool, "fetch", "-q", super, "+main:main")

	run(t, work, fromPool(superPool)...)
	if moved := run(t, work, "rev-parse", "refs/remotes/origin/main"); moved != run(t, super, "rev-parse", "HEAD") {
		t.Fatalf("origin/main is at %s, not at the commit that moved the submodule", moved)
	}
}
