//go:build windows

package workspace

// poolStillMounted has nothing to refuse on Windows, where the CSI driver does not run.
func poolStillMounted(string) error { return nil }
