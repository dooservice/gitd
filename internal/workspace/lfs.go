package workspace

import (
	"io"
	"os"
	"slices"
	"strings"
)

// gitd does not download git LFS objects: a workspace of a repository that uses LFS holds
// pointer files where those files are. Saying which, from the repository's .gitattributes,
// keeps an agent from wondering why a test cannot read a model or an image.

const (
	attributesFile = ".gitattributes"
	lfsAttribute   = "filter=lfs"
	// attributesLimit is the most of the file read: far more than any real one.
	attributesLimit = 1 << 20
)

// LFS are the patterns the workspace's .gitattributes sends to git LFS, in the order it
// names them; none when it names none or cannot be read. The file is read through an
// os.Root of the workspace, so a link there cannot make gitd read a file outside it.
func (workspaces *Workspaces) LFS(path string) []string {
	workspace, err := os.OpenRoot(path)
	if err != nil {
		return nil
	}
	defer workspace.Close()
	file, err := workspace.Open(attributesFile)
	if err != nil {
		return nil
	}
	defer file.Close()
	text, err := io.ReadAll(io.LimitReader(file, attributesLimit))
	if err != nil {
		return nil
	}
	return lfsPatterns(string(text))
}

// lfsPatterns are the patterns of attributes text whose attributes include filter=lfs.
func lfsPatterns(text string) []string {
	var patterns []string
	for line := range strings.Lines(text) {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if slices.Contains(fields[1:], lfsAttribute) && !slices.Contains(patterns, fields[0]) {
			patterns = append(patterns, fields[0])
		}
	}
	return patterns
}
