package strategy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These need CAP_SYS_ADMIN: CI runs them with sudo. Elsewhere they skip.
func needRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("mounting an overlay needs root")
	}
}

func TestTheKernelMountsAnOverlayWorkspaceOfAGitTemplateAndGitBehaves(t *testing.T) {
	needRoot(t)
	root := t.TempDir()
	template, dest := filepath.Join(root, "template"), filepath.Join(root, "agent-1")
	git := func(dir string, args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@a", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@a")
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(root, "init", "-q", "-b", "main", template)
	if err := os.WriteFile(filepath.Join(template, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(template, "add", ".")
	git(template, "commit", "-q", "-m", "first")
	git(template, "config", "core.checkStat", "minimal")
	git(template, "config", "core.trustctime", "false")

	maker := &OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: Kernel{}}
	if err := maker.Make(context.Background(), template, dest); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = maker.Unmake(context.Background(), dest) }()

	if status := git(dest, "status", "--porcelain"); status != "" {
		t.Errorf("a fresh workspace is dirty: %q", status)
	}
	if err := os.WriteFile(filepath.Join(dest, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(dest, "commit", "-qam", "agent")
	if data, _ := os.ReadFile(filepath.Join(template, "README.md")); string(data) != "hi\n" {
		t.Errorf("the template changed: %q", data)
	}
	if status := git(template, "status", "--porcelain"); status != "" {
		t.Errorf("the template is dirty: %q", status)
	}
}
