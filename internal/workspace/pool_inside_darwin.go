//go:build darwin

package workspace

// poolStillMounted has nothing to refuse on macOS, where the CSI driver does not run.
func poolStillMounted(string) error { return nil }
