package workspace

import "runtime"

// platformSettings are what a workspace or template on this system needs in its own
// configuration, for the agent's git as much as gitd's: on Windows, paths longer than 260
// characters, as deep trees such as node_modules have.
func platformSettings() []string {
	if runtime.GOOS == "windows" {
		return []string{"core.longpaths", "true"}
	}
	return nil
}
