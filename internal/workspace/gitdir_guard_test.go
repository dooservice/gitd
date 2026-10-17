package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

func TestAGitDirectoryLeadingOutOfTheWorkspaceIsRefused(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	for name, leadOut := range map[string]func(t *testing.T, gitDir string){
		"commondir": func(t *testing.T, gitDir string) {
			writeFile(t, filepath.Join(gitDir, "commondir"), t.TempDir()+"\n")
		},
		"objects as a link": func(t *testing.T, gitDir string) {
			elsewhere := filepath.Join(t.TempDir(), "objects")
			if err := os.Rename(filepath.Join(gitDir, "objects"), elsewhere); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, filepath.Join(gitDir, "objects")); err != nil {
				t.Fatal(err)
			}
		},
		"alternates naming another store": func(t *testing.T, gitDir string) {
			writeFile(t, filepath.Join(gitDir, "objects", "info", "alternates"), filepath.Join(t.TempDir(), "objects")+"\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			workspaces, roots := newTestWorkspaces(t, repository)
			path := made(t, workspaces, roots, "agent-1", repository)
			if err := workspaces.guard(context.Background(), path); err != nil {
				t.Fatalf("guard before: %v", err)
			}
			leadOut(t, filepath.Join(path, ".git"))
			if err := workspaces.guard(context.Background(), path); !refusedForConfig(err) {
				t.Fatalf("guard: %v; want CONFIG_REFUSED", err)
			}
		})
	}
}

func TestASubmoduleGitdNeverCheckedOutIsNotPublished(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	other := repo.Repo{Host: "github.com", Path: "acme/other"}
	workspaces, roots := newTestWorkspaces(t, repository, other)
	path := made(t, workspaces, roots, "agent-1", repository)
	writeFile(t, filepath.Join(path, ".gitmodules"), "[submodule \"lib\"]\n\tpath = lib\n\turl = "+other.HTTPS()+"\n")
	writeFile(t, filepath.Join(path, "lib", ".git"), "gitdir: ../.git/modules/lib\n")
	_, err := workspaces.stageSubmodules(context.Background(), path, repository, "agents/x")
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.PathRefused {
		t.Fatalf("stageSubmodules: %v; want PATH_REFUSED", err)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
