//go:build windows

package workspace

// isWhiteout is never true on Windows, which has no overlay to mark deletions.
func isWhiteout(string) bool { return false }
