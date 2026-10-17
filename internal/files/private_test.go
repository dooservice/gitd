package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAPrivateFileIsReplacedWholeAndReadableByItsOwnerOnly(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "kept.json")
	for _, content := range []string{"first", "second"} {
		if err := WritePrivate(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		read, err := os.ReadFile(path)
		if err != nil || string(read) != content {
			t.Fatalf("read %q, %v; want %q", read, err, content)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
}
