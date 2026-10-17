package git

import (
	"context"
	"slices"
	"strings"
)

// Version is what `git version` says.
func (runner *Runner) Version(ctx context.Context) (string, error) {
	out, err := runner.Run(ctx, Cmd{Dir: "/", Args: []string{"version"}})
	return strings.TrimSpace(out), err
}

// Resolve returns the commit a revision names in the repository at dir, and false when
// there is none. The revision comes from outside, so it goes after `--end-of-options`.
func (runner *Runner) Resolve(ctx context.Context, dir, revision string) (string, bool, error) {
	out, err := runner.Run(ctx, Cmd{Dir: dir, Args: []string{
		"rev-parse", "--verify", "--quiet", "--end-of-options", revision + "^{commit}",
	}})
	if failure, ok := AsFailure(err); ok && failure.ExitCode == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(out), true, nil
}

// ResolveObject is the object a revision names in the repository at dir — a blob as
// `<commit>:<path>` — and false when it names none.
func (runner *Runner) ResolveObject(ctx context.Context, dir, revision string) (string, bool, error) {
	out, err := runner.Run(ctx, Cmd{Dir: dir, Args: []string{"rev-parse", "--verify", "--quiet", "--end-of-options", revision}})
	if failure, ok := AsFailure(err); ok && failure.ExitCode == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(out), true, nil
}

// Branch is the branch the repository at dir is on, and false when it is on none.
func (runner *Runner) Branch(ctx context.Context, dir string) (string, bool) {
	out, err := runner.Run(ctx, Cmd{Dir: dir, Args: []string{"symbolic-ref", "--quiet", "--short", "HEAD"}})
	return strings.TrimSpace(out), err == nil
}

// Config reads one key from the git config file at path, and "" when it is not set.
func (runner *Runner) Config(ctx context.Context, path, key string) (string, error) {
	out, err := runner.Run(ctx, Cmd{Dir: "/", Args: []string{"config", "--file", path, "--get", "--end-of-options", key}})
	if failure, ok := AsFailure(err); ok && failure.ExitCode == 1 {
		return "", nil
	}
	return strings.TrimSpace(out), err
}

// ConfigEntry is one key of a git configuration file and its value. Key is as git lists it:
// section and name lowercased, a subsection as written.
type ConfigEntry struct {
	Key, Value string
}

// ConfigEntries lists every key of the configuration file at path, without following its
// includes. Listing runs nothing the file names.
func (runner *Runner) ConfigEntries(ctx context.Context, path string) ([]ConfigEntry, error) {
	out, err := runner.Run(ctx, Cmd{Dir: "/", Args: []string{"config", "--file", path, "--no-includes", "--null", "--list"}})
	if err != nil {
		return nil, err
	}
	var entries []ConfigEntry
	for _, listed := range strings.Split(out, "\x00") {
		if listed == "" {
			continue
		}
		key, value, _ := strings.Cut(listed, "\n")
		entries = append(entries, ConfigEntry{Key: key, Value: value})
	}
	return entries, nil
}

// SetConfig writes keys and their values into the repository at dir, in order.
func (runner *Runner) SetConfig(ctx context.Context, dir string, keysAndValues ...string) error {
	for pair := range slices.Chunk(keysAndValues, 2) {
		set := Cmd{Dir: dir, Args: []string{"config", "--end-of-options", pair[0], pair[1]}}
		if _, err := runner.Run(ctx, set); err != nil {
			return err
		}
	}
	return nil
}
