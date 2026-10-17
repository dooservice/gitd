package workspace

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

func TestWorksThatChangeTheSameLinesConflictAndOthersDoNot(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspacesOf(t, baseFiles, repository)
	var paths []string
	for index, change := range []struct{ file, content string }{
		{"src/app.go", "package app\n\nfunc One() int { return 2 }\n"},
		{"src/app.go", "package app\n\nfunc One() int { return 3 }\n"},
		{"README.md", "hello\n"},
	} {
		path := copied(t, workspaces, roots, "agent-"+string(rune('1'+index)), repository)
		write(t, filepath.Join(path, change.file), change.content)
		run(t, path, "commit", "-q", "-am", "agent's change")
		paths = append(paths, path)
	}
	sides := []*gitdv1.Side{{Of: &gitdv1.Side_Branch{Branch: "main"}}}
	for _, path := range paths {
		sides = append(sides, &gitdv1.Side{Of: &gitdv1.Side_Workspace{Workspace: path}})
	}
	answer, err := workspaces.Conflicts(context.Background(), &gitdv1.ConflictsRequest{Sides: sides, Url: repository.HTTPS()})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := answer.GetConflicts()
	if len(conflicts) != 1 || conflicts[0].GetFirst().GetWorkspace() != paths[0] ||
		conflicts[0].GetSecond().GetWorkspace() != paths[1] || !slices.Equal(conflicts[0].GetPaths(), []string{"src/app.go"}) {
		t.Errorf("conflicts = %v", conflicts)
	}
	if refs := run(t, workspaces.pools.Path(repository), "for-each-ref", "refs/gitd/brought/"); refs != "" {
		t.Errorf("what was brought into the pool stayed: %s", refs)
	}
}
