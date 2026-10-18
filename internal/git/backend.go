package git

import (
	"net/http/cgi"
	"os/exec"
)

// HTTPBackend is git's own smart HTTP server, `git http-backend`, over the repositories
// under root, run as gitd runs every git: its closed environment, no hooks and no global
// configuration. settings are `name=value` pairs set on its command line, where nothing in
// a repository can turn them back: which services it offers and their limits. The caller
// decides who may reach it and which repository; this only answers.
func (runner *Runner) HTTPBackend(root string, settings []string) *cgi.Handler {
	args := hardened(false)
	for _, setting := range settings {
		args = append(args, "-c", setting)
	}
	// CGI starts its program by an absolute path; the binary may be named by PATH alone.
	binary, err := exec.LookPath(runner.binary)
	if err != nil {
		binary = runner.binary
	}
	return &cgi.Handler{
		Path: binary,
		Args: append(args, "http-backend"),
		Env:  append(append([]string{}, runner.environment...), "GIT_PROJECT_ROOT="+root, "GIT_HTTP_EXPORT_ALL=1"),
	}
}
