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

	if _, err := workspaces.allowed(filepath.Join(root, "acme", "prod", "addons")); err != nil {
		t.Errorf("inside the root: %v", err)
	}
	for _, path := range []string{
		root,
		filepath.Join(root, "..", "elsewhere"),
		filepath.Join(root, "escape", "addons"),
		"relative/addons",
		"/etc/addons",
	} {
		if _, err := workspaces.allowed(path); err == nil {
			t.Errorf("allowed %q", path)
		}
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
