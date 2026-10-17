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
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// TestChangesReadFromALayerAgreeWithGit makes a workspace on the kernel's overlay, does in
// it what agents do — change, add, delete a file, delete a directory, remake one, commit —
// and checks that what Changes reads from the layer is what git says. Mounting takes root;
// CI runs it so.
func TestChangesReadFromALayerAgreeWithGit(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("mounting an overlay takes root")
	}
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	files := map[string]string{
		".gitignore": "node_modules/\n", "README.md": "hi\n", "src/app.go": "package app\n",
		"docs/old.md": "old\n", "docs/kept.md": "kept\n", "gone/a.md": "a\n", "gone/b.md": "b\n",
	}
	workspaces, roots := newTestWorkspacesOf(t, files, repository)
	layers := t.TempDir()
	workspaces.UseTemplates(template.New(filepath.Join(layers, "templates"), workspaces.locks, workspaces),
		&strategy.OverlayMaker{Layers: filepath.Join(layers, "layers"), Mounter: strategy.Kernel{}})
	path := made(t, workspaces, roots, "agent", repository)
	t.Cleanup(func() { _ = workspaces.Remove(context.Background(), &gitdv1.RemoveRequest{Path: path}) })

	write(t, filepath.Join(path, "src/app.go"), "package app\n\nfunc One() {}\n")
	run(t, path, "commit", "-q", "-am", "committed")
	write(t, filepath.Join(path, "src/new.go"), "package app\n")
	write(t, filepath.Join(path, "node_modules/x.js"), "ignored\n")
	write(t, filepath.Join(path, "README.md"), "hi\n")
	if err := os.RemoveAll(filepath.Join(path, "gone")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(path, "docs")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(path, "docs/kept.md"), "kept\n")

	answer, err := workspaces.Changes(context.Background(), &gitdv1.ChangesRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if !answer.GetFromLayer() {
		t.Fatal("the changes of an overlay workspace were not read from its layer")
	}
	var got []string
	for _, file := range answer.GetFiles() {
		got = append(got, strings.ToLower(strings.TrimPrefix(file.GetKind().String(), "KIND_"))+" "+file.GetPath())
	}
	want := fromGit(t, path, answer.GetBase())
	if !slices.Equal(got, want) {
		t.Errorf("from the layer:\n%v\nfrom git:\n%v", got, want)
	}
}

// fromGit is what git says changed in a workspace since base, committed or not, as
// "<kind> <path>" sorted by path.
func fromGit(t *testing.T, path, base string) []string {
	t.Helper()
	kinds := map[string]string{"A": "added", "M": "modified", "D": "deleted"}
	run(t, path, "add", "-A")
	var lines []string
	for line := range strings.Lines(run(t, path, "diff", "--cached", "--no-renames", "--name-status", base)) {
		kind, name, _ := strings.Cut(strings.TrimSpace(line), "\t")
		lines = append(lines, kinds[kind]+" "+name)
	}
	run(t, path, "reset", "-q")
	slices.SortFunc(lines, func(first, second string) int {
		return strings.Compare(first[strings.Index(first, " ")+1:], second[strings.Index(second, " ")+1:])
	})
	return lines
}
