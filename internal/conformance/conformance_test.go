package conformance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/strategy"
)

// maker makes and unmakes workspaces one way, in a directory where that way works.
type maker struct {
	name   string
	where  func(t *testing.T) string // a directory the template and workspaces live under
	make   func(ctx context.Context, template, dest string) error
	unmake func(ctx context.Context, dest string) error
}

func makers(t *testing.T) []maker {
	copied := maker{
		name:  "copy",
		where: func(t *testing.T) string { return t.TempDir() },
		make: func(_ context.Context, template, dest string) error {
			return exec.Command("cp", "-a", template, dest).Run()
		},
		unmake: func(_ context.Context, dest string) error { return os.RemoveAll(dest) },
	}
	makers := []maker{copied}

	if os.Geteuid() == 0 {
		var overlay *strategy.OverlayMaker
		makers = append(makers, maker{
			name: "overlay",
			where: func(t *testing.T) string {
				dir := t.TempDir()
				overlay = &strategy.OverlayMaker{Layers: filepath.Join(dir, "layers"), Mounter: strategy.Kernel{}}
				return dir
			},
			make:   func(ctx context.Context, template, dest string) error { return overlay.Make(ctx, template, dest) },
			unmake: func(ctx context.Context, dest string) error { return overlay.Unmake(ctx, dest) },
		})
	}
	if xfs := os.Getenv("GITD_CONFORMANCE_XFS"); xfs != "" {
		makers = append(makers, maker{
			name: "reflink",
			where: func(t *testing.T) string {
				dir, err := os.MkdirTemp(xfs, "conformance-")
				must(t, err)
				t.Cleanup(func() { _ = os.RemoveAll(dir) })
				return dir
			},
			make:   strategy.ReflinkMaker{}.Make,
			unmake: strategy.ReflinkMaker{}.Unmake,
		})
	}
	return makers
}

func TestEveryStrategyBehavesUnderWhatAnAgentDoes(t *testing.T) {
	for _, way := range makers(t) {
		t.Run(way.name, func(t *testing.T) {
			base := way.where(t)
			upstream, template := filepath.Join(base, "upstream"), filepath.Join(base, "template")
			main, feature := makeUpstream(t, upstream)
			git(t, base, "clone", "-q", upstream, template)
			git(t, template, "config", "core.checkStat", "minimal")
			git(t, template, "config", "core.trustctime", "false")
			before := fingerprint(t, template)

			ctx := context.Background()
			first, second := filepath.Join(base, "agent-1"), filepath.Join(base, "agent-2")
			for _, dest := range []string{first, second} {
				must(t, way.make(ctx, template, dest))
			}
			t.Cleanup(func() {
				for _, dest := range []string{first, second} {
					_ = way.unmake(ctx, dest)
				}
			})

			agentWork(t, first, main, feature)

			if status(t, second) != "" {
				t.Errorf("the other workspace saw the first one's changes: %q", status(t, second))
			}
			if read(t, filepath.Join(second, "README.md")) != "hello\n" {
				t.Error("the other workspace's file changed")
			}

			pool := filepath.Join(base, "pool.git")
			git(t, base, "init", "-q", "--bare", pool)
			git(t, pool, "fetch", "-q", first, "agent/work:refs/heads/agent/work")
			git(t, pool, "fsck", "--no-progress")

			if after := fingerprint(t, template); after != before {
				t.Errorf("the template changed:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

// agentWork is what an agent does in a workspace, checking git sees each step as it is.
func agentWork(t *testing.T, ws, main, feature string) {
	t.Helper()
	if got := status(t, ws); got != "" {
		t.Fatalf("a fresh workspace is dirty: %q", got)
	}
	if head := git(t, ws, "rev-parse", "HEAD"); head != main {
		t.Fatalf("HEAD is %s, not %s", head, main)
	}
	write(t, filepath.Join(ws, "README.md"), "hello agent\n")
	expect(t, status(t, ws), " M README.md", "after an edit")
	if !strings.Contains(git(t, ws, "diff"), "+hello agent") {
		t.Error("diff misses the edit")
	}
	write(t, filepath.Join(ws, "src", "new.go"), "package src\n")
	expect(t, status(t, ws), " M README.md\n?? src/new.go", "after a new file")
	must(t, os.Remove(filepath.Join(ws, "src", "old.go")))
	must(t, os.Chmod(filepath.Join(ws, "run.sh"), 0o644))
	expect(t, status(t, ws), " M README.md\n M run.sh\n D src/old.go\n?? src/new.go", "after a delete and a mode change")

	git(t, ws, "stash", "push", "-q", "--include-untracked")
	expect(t, status(t, ws), "", "after stash")
	expect(t, read(t, filepath.Join(ws, "README.md")), "hello\n", "README after stash")
	git(t, ws, "stash", "pop", "-q")
	expect(t, status(t, ws), " M README.md\n M run.sh\n D src/old.go\n?? src/new.go", "after stash pop")

	git(t, ws, "checkout", "-q", "-b", "agent/work")
	git(t, ws, "add", "-A")
	git(t, ws, "commit", "-q", "-m", "agent work")
	expect(t, status(t, ws), "", "after commit")
	expect(t, git(t, ws, "log", "-1", "--format=%s"), "agent work", "the commit")

	git(t, ws, "checkout", "-q", feature)
	expect(t, read(t, filepath.Join(ws, "FEATURE.md")), "feature\n", "a file of the other branch")
	if _, err := os.Stat(filepath.Join(ws, "src", "new.go")); err == nil {
		t.Error("the agent's file shows on another branch")
	}
	git(t, ws, "checkout", "-q", "agent/work")
	expect(t, read(t, filepath.Join(ws, "src", "new.go")), "package src\n", "the agent's file back on its branch")

	git(t, ws, "rebase", "-q", feature)
	expect(t, read(t, filepath.Join(ws, "FEATURE.md")), "feature\n", "after rebase onto the feature")
	expect(t, status(t, ws), "", "after rebase")

	git(t, ws, "reset", "-q", "--hard", "HEAD~1")
	if _, err := os.Stat(filepath.Join(ws, "src", "new.go")); err == nil {
		t.Error("reset --hard kept the agent's file")
	}
	git(t, ws, "reset", "-q", "--hard", "ORIG_HEAD")
	git(t, ws, "gc", "-q", "--prune=now")
	git(t, ws, "fsck", "--no-progress")
	expect(t, status(t, ws), "", "after gc")
	if !strings.Contains(git(t, ws, "blame", "README.md"), "hello agent") {
		t.Error("blame misses the agent's line")
	}
}

// makeUpstream makes a repository with history, two branches, a subdirectory, an
// executable, a symbolic link and a binary file, and returns its two branch tips.
func makeUpstream(t *testing.T, dir string) (main, feature string) {
	t.Helper()
	git(t, filepath.Dir(dir), "init", "-q", "-b", "main", dir)
	write(t, filepath.Join(dir, "README.md"), "hello\n")
	write(t, filepath.Join(dir, "src", "old.go"), "package src\n")
	write(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\necho run\n")
	must(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o755))
	must(t, os.Symlink("src/old.go", filepath.Join(dir, "link")))
	must(t, os.WriteFile(filepath.Join(dir, "blob.bin"), []byte{0, 1, 2, 255, 254, 0}, 0o644))
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "first")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "second")
	git(t, dir, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(dir, "FEATURE.md"), "feature\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "feature")
	feature = git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", "main")
	return git(t, dir, "rev-parse", "HEAD"), feature
}

// fingerprint is a hash of every file under dir — path, mode, and content or link — so
// any change to a template shows.
func fingerprint(t *testing.T, dir string) string {
	t.Helper()
	var entries []string
	must(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(dir, path)
		line := relative + " " + info.Mode().String()
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, _ := os.Readlink(path)
			line += " -> " + target
		case info.Mode().IsRegular():
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			sum := sha256.New()
			_, err = io.Copy(sum, file)
			file.Close()
			if err != nil {
				return err
			}
			line += " " + hex.EncodeToString(sum.Sum(nil))
		}
		entries = append(entries, line)
		return nil
	}))
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(gitRaw(t, dir, args...))
}

// gitRaw is git's output as it is: porcelain status starts lines with a space.
func gitRaw(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*",
		"GIT_CONFIG_KEY_1=protocol.file.allow", "GIT_CONFIG_VALUE_1=always",
		"GIT_AUTHOR_NAME=agent", "GIT_AUTHOR_EMAIL=agent@example.com",
		"GIT_COMMITTER_NAME=agent", "GIT_COMMITTER_EMAIL=agent@example.com")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func status(t *testing.T, ws string) string {
	t.Helper()
	return strings.TrimRight(gitRaw(t, ws, "status", "--porcelain"), "\n")
}

func expect(t *testing.T, got, want, what string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", what, got, want)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(text), 0o644))
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	must(t, err)
	return string(data)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
