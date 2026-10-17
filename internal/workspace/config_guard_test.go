package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/git"
)

// guarded is a workspace-like repository with a guard to check it.
func guarded(t *testing.T) (*Workspaces, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent-1")
	run(t, filepath.Dir(path), "init", "-q", "-b", "main", path)
	run(t, path, "config", "gitd.url", "https://github.com/acme/web")
	run(t, path, "config", "user.name", "agent")
	return &Workspaces{runner: git.NewRunner("git")}, path
}

func refusedForConfig(err error) bool {
	failure, ok := git.AsFailure(err)
	return ok && failure.Reason == git.ConfigRefused
}

func TestAWorkspaceWithOnlyTheKeysGitdAllowsIsTrusted(t *testing.T) {
	workspaces, path := guarded(t)
	run(t, path, "config", "remote.origin.url", "https://github.com/acme/web")
	run(t, path, "config", "branch.feature/x.y.remote", "origin")
	run(t, path, "config", "alias.st", "status")
	if err := workspaces.guard(context.Background(), path); err != nil {
		t.Fatalf("guard: %v", err)
	}
}

func TestAKeyThatRunsACommandIsRefused(t *testing.T) {
	for _, key := range []string{
		"filter.evil.clean", "filter.evil.smudge", "core.hookspath", "core.fsmonitor", "core.sshcommand",
		"core.worktree", "include.path", "remote.origin.uploadpack", "submodule.lib.update", "diff.x.textconv",
		"extensions.worktreeconfig",
	} {
		t.Run(key, func(t *testing.T) {
			workspaces, path := guarded(t)
			run(t, path, "config", key, "true")
			if err := workspaces.guard(context.Background(), path); !refusedForConfig(err) {
				t.Fatalf("guard with %s: %v; want CONFIG_REFUSED", key, err)
			}
		})
	}
}

func TestAGitDirectoryThatIsALinkIsRefused(t *testing.T) {
	workspaces, path := guarded(t)
	elsewhere := filepath.Join(t.TempDir(), "real.git")
	if err := os.Rename(filepath.Join(path, ".git"), elsewhere); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(path, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.guard(context.Background(), path); !refusedForConfig(err) {
		t.Fatalf("guard: %v; want CONFIG_REFUSED", err)
	}
}

func TestASubmoduleMayPointOnlyAtItsCheckoutInsideTheWorkspace(t *testing.T) {
	for workTree, trusted := range map[string]bool{
		"../../../lib":          true,
		"../../../../outside":   false,
		"/etc":                  false,
		"../..":                 false,
		"../../../.git/objects": false,
	} {
		t.Run(workTree, func(t *testing.T) {
			workspaces, path := guarded(t)
			module := filepath.Join(path, ".git", "modules", "lib")
			run(t, path, "init", "-q", "--bare", module)
			run(t, module, "config", "core.worktree", workTree)
			err := workspaces.guard(context.Background(), path)
			if trusted && err != nil || !trusted && !refusedForConfig(err) {
				t.Fatalf("core.worktree %s: %v; trusted %v", workTree, err, trusted)
			}
		})
	}
}

func TestAPathInsideAnotherWorkspaceIsRefused(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "agent-1")
	run(t, root, "init", "-q", workspace)
	workspaces := New([]string{root}, git.NewRunner("git"), nil, nil, nil, 0)
	if _, err := workspaces.allowed(filepath.Join(workspace, "nested", "agent-2")); err == nil {
		t.Fatal("a path inside a workspace is allowed")
	}
	if _, err := workspaces.allowed(filepath.Join(root, "agent-2")); err != nil {
		t.Fatalf("a path beside it: %v", err)
	}
}

func TestEveryGroupOfAllowedKeysSaysWhyAndMatchesItsKeys(t *testing.T) {
	for _, group := range allowedKeyGroups {
		if group.why == "" {
			t.Errorf("a group of %d keys says not why they are allowed", len(group.keys))
		}
		for _, key := range group.keys {
			listed := strings.ReplaceAll(key, "<subsection>", "any.thing")
			if !allowedKeys.MatchString(listed) {
				t.Errorf("%s is listed but not allowed", key)
			}
		}
	}
}

func TestASubmoduleLinkedOutOfTheWorkspaceGetsNothingWritten(t *testing.T) {
	workspaces, path := guarded(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(path, "lib")); err != nil {
		t.Fatal(err)
	}
	workspace, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	if err := workspace.MkdirAll(filepath.Join(".git", "modules", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	err = workspaces.pointAt(context.Background(), workspace, filepath.Join(".git", "modules", "lib"), "lib")
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.PathRefused {
		t.Fatalf("pointAt through a link: %v; want PATH_REFUSED", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("%d files written outside the workspace", len(entries))
	}
}
