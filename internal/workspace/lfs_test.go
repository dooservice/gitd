package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTheFilesSentToLFSAreNamed(t *testing.T) {
	text := "# models\n*.onnx filter=lfs diff=lfs merge=lfs -text\n*.txt text\n\nassets/** filter=lfs -text\n*.onnx filter=lfs\n"
	if got := lfsPatterns(text); !slices.Equal(got, []string{"*.onnx", "assets/**"}) {
		t.Fatalf("lfsPatterns = %q", got)
	}
}

func TestAnAttributesFileLinkedOutOfTheWorkspaceIsNotRead(t *testing.T) {
	workspaces, path := guarded(t)
	outside := filepath.Join(t.TempDir(), "attributes")
	if err := os.WriteFile(outside, []byte("*.secret filter=lfs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(path, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	if got := workspaces.LFS(path); len(got) != 0 {
		t.Fatalf("read %q through a link out of the workspace", got)
	}
	if err := os.Remove(filepath.Join(path, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".gitattributes"), []byte("*.png filter=lfs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := workspaces.LFS(path); !slices.Equal(got, []string{"*.png"}) {
		t.Fatalf("LFS = %q", got)
	}
}
