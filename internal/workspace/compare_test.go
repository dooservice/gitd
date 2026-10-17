package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/repo"
)

// kindsOf are compared files as "<kind> <path>".
func kindsOf(files []*gitdv1.ChangedFile) []string {
	var listed []string
	for _, file := range files {
		listed = append(listed, strings.ToLower(strings.TrimPrefix(file.GetKind().String(), "KIND_"))+" "+file.GetPath())
	}
	return listed
}

func TestTwoAgentsWorkIsComparedFileByFile(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspacesOf(t, baseFiles, repository)
	first, second := copied(t, workspaces, roots, "agent-1", repository), copied(t, workspaces, roots, "agent-2", repository)
	write(t, filepath.Join(first, "src/app.go"), "package app\n\nfunc One() int { return 1 }\nfunc Mine() {}\n")
	write(t, filepath.Join(first, "a.txt"), "only the first\n")
	write(t, filepath.Join(second, "src/app.go"), "package app\n\nfunc One() int { return 3 }\n")
	run(t, second, "commit", "-q", "-am", "second's change")
	if err := os.Remove(filepath.Join(second, "docs/old.md")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	answer, err := workspaces.Compare(ctx, &gitdv1.CompareRequest{
		First: &gitdv1.Side{Of: &gitdv1.Side_Workspace{Workspace: first}}, Second: &gitdv1.Side{Of: &gitdv1.Side_Workspace{Workspace: second}},
		WithDiff: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"deleted a.txt", "deleted docs/old.md", "modified src/app.go"}
	if got := kindsOf(answer.GetFiles()); !slices.Equal(got, want) {
		t.Errorf("compared = %v; want %v", got, want)
	}
	if !strings.Contains(answer.GetDiff(), "-func Mine() {}\n") || !strings.Contains(answer.GetDiff(), "+func One() int { return 3 }\n") {
		t.Errorf("diff:\n%s", answer.GetDiff())
	}

	againstMain, err := workspaces.Compare(ctx, &gitdv1.CompareRequest{
		First: &gitdv1.Side{Of: &gitdv1.Side_Workspace{Workspace: first}}, Second: &gitdv1.Side{Of: &gitdv1.Side_Branch{Branch: "main"}},
		Url: repository.HTTPS(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := kindsOf(againstMain.GetFiles()); !slices.Equal(got, []string{"deleted a.txt", "modified src/app.go"}) {
		t.Errorf("against main = %v", got)
	}
}

func TestWorksOfDifferentRepositoriesAreNotCompared(t *testing.T) {
	web, tools := repo.Repo{Host: "github.com", Path: "acme/web"}, repo.Repo{Host: "github.com", Path: "acme/tools"}
	workspaces, roots := newTestWorkspacesOf(t, baseFiles, web, tools)
	first := copied(t, workspaces, roots, "agent-1", web)
	_, err := workspaces.Compare(context.Background(), &gitdv1.CompareRequest{
		First:  &gitdv1.Side{Of: &gitdv1.Side_Workspace{Workspace: first}},
		Second: &gitdv1.Side{Of: &gitdv1.Side_Branch{Branch: "main"}}, Url: tools.HTTPS(),
	})
	if err == nil {
		t.Error("works of two repositories were compared")
	}
}
