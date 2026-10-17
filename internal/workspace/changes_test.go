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

// baseFiles are the first commit of the repositories of these tests.
var baseFiles = map[string]string{
	".gitignore":   "node_modules/\n",
	"README.md":    "hi\n",
	"src/app.go":   "package app\n\nfunc One() int { return 1 }\n",
	"docs/old.md":  "old\n",
	"tests/x_test": "test\n",
}

// copied is a workspace made by writing its files, on main.
func copied(t *testing.T, workspaces *Workspaces, roots, name string, repository repo.Repo) string {
	t.Helper()
	path := filepath.Join(roots, name)
	if _, err := workspaces.create(context.Background(), job{path: path, repository: repository, revision: onBranch("main")}); err != nil {
		t.Fatal(err)
	}
	return path
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangesAreWhatTheAgentDidSinceGitdPutTheWorkspaceOnItsCommit(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspacesOf(t, baseFiles, repository)
	path := copied(t, workspaces, roots, "agent", repository)

	write(t, filepath.Join(path, "src/app.go"), "package app\n\nfunc One() int { return 2 }\n\nfunc Two() int { return 2 }\n")
	run(t, path, "commit", "-q", "-am", "committed change")
	write(t, filepath.Join(path, "src/new.go"), "package app\n")
	write(t, filepath.Join(path, "node_modules/left.js"), "ignored\n")
	write(t, filepath.Join(path, "README.md"), "hi\n") // touched, the same
	if err := os.Remove(filepath.Join(path, "docs/old.md")); err != nil {
		t.Fatal(err)
	}

	answer, err := workspaces.Changes(context.Background(), &gitdv1.ChangesRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*gitdv1.ChangedFile{}
	var names []string
	for _, file := range answer.GetFiles() {
		got[file.GetPath()] = file
		names = append(names, file.GetPath())
	}
	if !slices.Equal(names, []string{"docs/old.md", "src/app.go", "src/new.go"}) {
		t.Fatalf("changed = %v", names)
	}
	if app := got["src/app.go"]; app.GetKind() != gitdv1.ChangedFile_KIND_MODIFIED || app.GetAddedLines() != 3 || app.GetRemovedLines() != 1 {
		t.Errorf("src/app.go = %v", app)
	}
	if old := got["docs/old.md"]; old.GetKind() != gitdv1.ChangedFile_KIND_DELETED || old.GetRemovedLines() != 1 {
		t.Errorf("docs/old.md = %v", old)
	}
	if added := got["src/new.go"]; added.GetKind() != gitdv1.ChangedFile_KIND_ADDED || added.GetAddedLines() != 1 {
		t.Errorf("src/new.go = %v", added)
	}
	if answer.GetFromLayer() || answer.GetBase() == "" {
		t.Errorf("answer = %v", answer)
	}

	withIgnored, err := workspaces.Changes(context.Background(), &gitdv1.ChangesRequest{Path: path, IncludeIgnored: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withIgnored.GetFiles()) != 4 {
		t.Errorf("with ignored files: %v", withIgnored.GetFiles())
	}
}

func TestALayerSaysWhatItsAgentTouchedAndWhatItHides(t *testing.T) {
	root := t.TempDir()
	lower, upper, merged := filepath.Join(root, "lower"), filepath.Join(root, "upper"), filepath.Join(root, "merged")
	for name, content := range map[string]string{"src/app.go": "a", "docs/old.md": "old", "docs/kept.md": "kept"} {
		write(t, filepath.Join(lower, name), content)
		write(t, filepath.Join(merged, name), content)
	}
	// The agent changed src/app.go, added src/new.go, and remade docs/ with kept.md only.
	write(t, filepath.Join(upper, "src/app.go"), "b")
	write(t, filepath.Join(upper, "src/new.go"), "new")
	write(t, filepath.Join(upper, "docs/kept.md"), "kept")
	write(t, filepath.Join(upper, ".git/index"), "index")
	if err := os.Remove(filepath.Join(merged, "docs/old.md")); err != nil {
		t.Fatal(err)
	}
	view, err := os.OpenRoot(merged)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	candidates, err := layerPaths(lower, upper, view)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(candidates)
	if !slices.Equal(candidates, []string{"docs/kept.md", "docs/old.md", "src/app.go", "src/new.go"}) {
		t.Errorf("candidates = %v", candidates)
	}
}

func TestADiffIsGitsOwnOfTheChangesAndStopsAtItsSize(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspacesOf(t, baseFiles, repository)
	path := copied(t, workspaces, roots, "agent", repository)
	write(t, filepath.Join(path, "src/app.go"), "package app\n\nfunc One() int { return 2 }\n")
	write(t, filepath.Join(path, "src/new.go"), "package app\n")
	if err := os.Remove(filepath.Join(path, "docs/old.md")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	answer, err := workspaces.Diff(ctx, &gitdv1.DiffRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"diff --git a/src/app.go b/src/app.go\n", "--- a/src/app.go\n+++ b/src/app.go\n",
		"-func One() int { return 1 }\n+func One() int { return 2 }\n",
		"diff --git a/src/new.go b/src/new.go\nnew file mode 100644\n", "--- /dev/null\n+++ b/src/new.go\n",
		"diff --git a/docs/old.md b/docs/old.md\ndeleted file mode 100644\n", "--- a/docs/old.md\n+++ /dev/null\n",
	} {
		if !strings.Contains(answer.GetDiff(), want) {
			t.Errorf("the diff lacks %q:\n%s", want, answer.GetDiff())
		}
	}
	if answer.GetTruncated() {
		t.Error("a small diff was cut")
	}
	one, err := workspaces.Diff(ctx, &gitdv1.DiffRequest{Path: path, Files: []string{"src/new.go"}})
	if err != nil || strings.Contains(one.GetDiff(), "src/app.go") || !strings.Contains(one.GetDiff(), "src/new.go") {
		t.Errorf("one file: %v\n%s", err, one.GetDiff())
	}
	cut, err := workspaces.Diff(ctx, &gitdv1.DiffRequest{Path: path, MaxBytes: 40})
	if err != nil || !cut.GetTruncated() || len(cut.GetDiff()) != 40 {
		t.Errorf("cut: %v, %d bytes, truncated %v", err, len(cut.GetDiff()), cut.GetTruncated())
	}
}
