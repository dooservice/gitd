package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
	"golang.org/x/sys/unix"
)

// TestAWarmLayerCarriesWhatOneWorkspaceInstalledToTheNext seals, on the kernel's overlay,
// a workspace where dependencies were installed — files git ignores written, a lock file
// rewritten, a file deleted — and makes another workspace on it: it holds all of that from
// the start, git sees it as the first did, and Changes agrees with git. Mounting takes
// root; CI runs it so.
func TestAWarmLayerCarriesWhatOneWorkspaceInstalledToTheNext(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("mounting an overlay takes root")
	}
	ctx := context.Background()
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	files := map[string]string{
		".gitignore": "node_modules/\n", "package.json": "{}\n", "package-lock.json": "v1\n",
		"README.md": "hi\n", "docs/old.md": "old\n",
	}
	workspaces, roots := newTestWorkspacesOf(t, files, repository)
	state := t.TempDir()
	templates := template.New(filepath.Join(state, "templates"), workspaces.locks, workspaces)
	workspaces.UseTemplates(templates, &strategy.OverlayMaker{Layers: filepath.Join(state, "layers"), Mounter: strategy.Kernel{}})

	installed := made(t, workspaces, roots, "agent-0", repository)
	write(t, filepath.Join(installed, "node_modules/dep/index.js"), "module.exports = 1\n")
	write(t, filepath.Join(installed, "package-lock.json"), "v2\n")
	if err := os.Remove(filepath.Join(installed, "docs/old.md")); err != nil {
		t.Fatal(err)
	}
	run(t, installed, "checkout", "-q", "-b", "installing")

	// A sandbox still binding the workspace would write into the layer after it was moved.
	sandbox := t.TempDir()
	if err := unix.Mount(installed, sandbox, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := workspaces.Seal(ctx, &gitdv1.SealRequest{Path: installed, Name: "deps"}); !hasReason(err, git.Busy) {
		t.Errorf("sealed a workspace a sandbox still binds: %v", err)
	}
	if head := run(t, installed, "symbolic-ref", "HEAD"); head != "refs/heads/installing" {
		t.Errorf("a workspace whose sealing was refused was not put back: HEAD is %s", head)
	}
	if err := unix.Unmount(sandbox, 0); err != nil {
		t.Fatal(err)
	}
	sealed, err := workspaces.Seal(ctx, &gitdv1.SealRequest{Path: installed, Name: "deps"})
	if err != nil {
		t.Fatal(err)
	}
	if sealed.GetChangedFiles() != 2 || sealed.GetName() != "deps" || sealed.GetSealedFrom() != installed {
		t.Errorf("sealed = %+v; want package-lock.json and docs/old.md changed", sealed)
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Errorf("the sealed workspace is still there: %v", err)
	}

	path := filepath.Join(roots, "agent-1")
	t.Cleanup(func() { _ = workspaces.Remove(context.Background(), &gitdv1.RemoveRequest{Path: path}) })
	result, err := workspaces.createFromTemplate(ctx, job{path: path, repository: repository, revision: onBranch("main"), warm: "deps"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Layers.GetWarm() == "" || result.Branch != "main" {
		t.Errorf("result = %+v; want main, on the warm layer", result)
	}
	if data, err := os.ReadFile(filepath.Join(path, "node_modules/dep/index.js")); err != nil || string(data) != "module.exports = 1\n" {
		t.Errorf("the installed dependency is not there: %q, %v", data, err)
	}
	if head := run(t, path, "symbolic-ref", "HEAD"); head != "refs/heads/main" {
		t.Errorf("HEAD is %s: the sealed workspace's .git came along", head)
	}
	if status := run(t, path, "status", "--porcelain"); status != "D docs/old.md\n M package-lock.json" && status != " D docs/old.md\n M package-lock.json" {
		t.Errorf("git status = %q", status)
	}
	write(t, filepath.Join(path, "src/new.go"), "package app\n")
	answer, err := workspaces.Changes(ctx, &gitdv1.ChangesRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, file := range answer.GetFiles() {
		got = append(got, strings.ToLower(strings.TrimPrefix(file.GetKind().String(), "KIND_"))+" "+file.GetPath())
	}
	if want := fromGit(t, path, answer.GetBase()); !slices.Equal(got, want) {
		t.Errorf("from the layers:\n%v\nfrom git:\n%v", got, want)
	}

	if _, err := workspaces.Seal(ctx, &gitdv1.SealRequest{Path: path, Name: "more"}); !hasReason(err, git.Unsupported) {
		t.Errorf("sealed a workspace standing on a warm layer: %v", err)
	}
	listed, err := workspaces.ListWarm(&gitdv1.ListWarmRequest{Url: repository.HTTPS()})
	if err != nil || len(listed.GetWarm()) != 1 || listed.GetWarm()[0].GetUsers() != 1 {
		t.Errorf("listed = %+v, %v", listed, err)
	}
	other := made(t, workspaces, roots, "agent-2", repository)
	if _, err := workspaces.Seal(ctx, &gitdv1.SealRequest{Path: other, Name: "deps"}); !hasReason(err, git.Busy) {
		t.Errorf("replaced a warm layer a workspace stands on: %v", err)
	}
	if status := run(t, other, "status", "--porcelain"); status != "" {
		t.Errorf("a workspace whose sealing was refused was not put back: %q", status)
	}
	if err := workspaces.Remove(ctx, &gitdv1.RemoveRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	if listed, _ := workspaces.ListWarm(&gitdv1.ListWarmRequest{Url: repository.HTTPS()}); listed.GetWarm()[0].GetUsers() != 0 {
		t.Errorf("a removed workspace still stands on the warm layer: %+v", listed)
	}
	if _, err := workspaces.Seal(ctx, &gitdv1.SealRequest{Path: other, Name: "deps"}); err != nil {
		t.Errorf("a warm layer no workspace stands on was not replaced: %v", err)
	}
}

func hasReason(err error, reason git.Reason) bool {
	failure, ok := git.AsFailure(err)
	return ok && failure.Reason == reason
}
