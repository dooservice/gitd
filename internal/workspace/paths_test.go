package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnlyWhatLiesUnderARootIsAllowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	workspaces := &Workspaces{roots: []string{root}}

	if _, err := workspaces.allowed(filepath.Join(root, "acme", "prod", "app")); err != nil {
		t.Errorf("inside the root: %v", err)
	}
	for _, path := range []string{
		root,
		filepath.Join(root, "..", "elsewhere"),
		filepath.Join(root, "escape", "app"),
		"relative/app",
		"/etc/app",
	} {
		if _, err := workspaces.allowed(path); err == nil {
			t.Errorf("allowed %q", path)
		}
	}
}

func TestARootReachedThroughALinkStillHoldsItsWorkspaces(t *testing.T) {
	real := t.TempDir()
	linked := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(real, linked); err != nil {
		t.Fatal(err)
	}
	workspaces := &Workspaces{roots: []string{linked}}
	if _, err := workspaces.allowed(filepath.Join(linked, "agent-1")); err != nil {
		t.Errorf("a workspace under a root that is a link: %v", err)
	}
	if _, err := workspaces.allowed(filepath.Join(real, "..", "elsewhere")); err == nil {
		t.Error("a path beside the root's target was allowed")
	}
}

func TestOnlyNothingOrAnEmptyDirectoryIsVacant(t *testing.T) {
	dir := t.TempDir()
	if !vacant(filepath.Join(dir, "missing")) || !vacant(dir) {
		t.Error("an empty place is not vacant")
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if vacant(dir) {
		t.Error("a directory with a file is vacant")
	}
}

func TestARootThatIsALinkStillAllowsWhatLiesUnderIt(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspaces")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	workspaces := New([]string{link}, nil, nil, nil, nil, 0)
	if _, err := workspaces.allowed(filepath.Join(link, "acme", "app")); err != nil {
		t.Errorf("under a linked root: %v", err)
	}
}
