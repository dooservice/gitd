package git

import "os"

// subcommands are everything gitd runs git for, and nothing else can be run.
func subcommands() map[string]bool {
	return map[string]bool{
		"init":             true,
		"push":             true,
		"update-ref":       true,
		"check-ref-format": true,
		"clone":            true,
		"fetch":            true,
		"ls-remote":        true,
		"rev-parse":        true,
		"checkout":         true,
		"submodule":        true,
		"config":           true,
		"symbolic-ref":     true,
		"ls-tree":          true,
		"log":              true,
		"status":           true,
		"count-objects":    true,
		"repack":           true,
		"pack-refs":        true,
		"commit-graph":     true,
		"fsck":             true,
		"version":          true,
	}
}

// hardened is set on the command line, where the repository being read cannot turn any
// of it back on: no hooks, no fsmonitor, no credential helper, no prompt, no transport but
// HTTPS and SSH, and no HTTP redirect, so a token sent to a repository's host goes there
// only: a moved repository fails with where it went, rather than being followed.
func hardened(local bool) []string {
	file := "protocol.file.allow=never"
	if local {
		file = "protocol.file.allow=always"
	}
	return []string{
		"-c", "credential.helper=",
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.fsmonitor=false",
		"-c", "core.askPass=",
		"-c", "http.followRedirects=false",
		"-c", "protocol.allow=never",
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.ssh.allow=always",
		"-c", "protocol.https.allow=always",
		"-c", file,
	}
}

// closedEnvironment is the only environment git sees: the little it needs from gitd's own,
// and global and system configuration switched off.
func closedEnvironment() []string {
	environment := []string{
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"XDG_CONFIG_HOME=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	}
	for _, name := range []string{"HOME", "PATH", "LD_LIBRARY_PATH", "TZ"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}
