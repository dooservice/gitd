//go:build linux

package mounthelper

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyADirectoryBeneathItsBaseReachedWithoutSymlinksIsPinned(t *testing.T) {
	base := t.TempDir()
	elsewhere := t.TempDir()
	inside := filepath.Join(base, "a", "b")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "a"), filepath.Join(base, "hop")); err != nil {
		t.Fatal(err)
	}
	uid := os.Getuid()

	dir, err := pin(base, inside, uid)
	if err != nil {
		t.Fatalf("pin(inside) = %v", err)
	}
	if target, _ := os.Readlink(dir.Path); target != inside {
		t.Errorf("pinned %s, not %s", target, inside)
	}
	dir.Close()

	for _, path := range []string{
		base,                               // the base itself
		filepath.Join(base, "link"),        // a symbolic link out
		filepath.Join(base, "hop", "b"),    // through a symbolic link, even one that stays inside
		filepath.Join(base, "..", "x"),     // not clean
		elsewhere,                          // outside
		"relative/a",                       // relative
		filepath.Join(base, "a", "absent"), // not there
	} {
		if dir, err := pin(base, path, uid); !errors.Is(err, errRefused) {
			dir.Close()
			t.Errorf("pin(%q) = %v; want refused", path, err)
		}
	}
	if _, err := pin(base, inside, uid+1); !errors.Is(err, errRefused) {
		t.Errorf("a directory of another user was pinned: %v", err)
	}
}

func TestARequestFromAnotherUserOrForAnythingElseIsRefused(t *testing.T) {
	config := Config{UID: 1500, Templates: []string{"/t"}, Layers: []string{"/l"}, Roots: []string{"/r"}}
	if err := Do(config, 1501, Request{Op: "mount"}); !errors.Is(err, errRefused) {
		t.Errorf("another user: %v", err)
	}
	if err := Do(config, 1500, Request{Op: "chmod", Target: "/r/x"}); !errors.Is(err, errRefused) {
		t.Errorf("another operation: %v", err)
	}
	if err := Do(config, 1500, Request{Op: "mount", Lower: "/etc", Upper: "/l/u", Work: "/l/w", Target: "/r/x"}); !errors.Is(err, errRefused) {
		t.Errorf("a lower layer outside the templates: %v", err)
	}
}

func FuzzAMountIsNeverCheckedOutsideItsDirectories(f *testing.F) {
	base := f.TempDir()
	config := Config{UID: os.Getuid(), Templates: []string{filepath.Join(base, "t")}, Layers: []string{filepath.Join(base, "l")}, Roots: []string{filepath.Join(base, "r")}}
	for _, dir := range []string{filepath.Join(config.Templates[0], "web"), filepath.Join(config.Layers[0], "u"), filepath.Join(config.Layers[0], "w"), filepath.Join(config.Roots[0], "agent")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			f.Fatal(err)
		}
	}
	if err := os.Symlink("/etc", filepath.Join(config.Templates[0], "out")); err != nil {
		f.Fatal(err)
	}
	f.Add(filepath.Join(config.Templates[0], "web"), filepath.Join(config.Layers[0], "u"), filepath.Join(config.Layers[0], "w"), filepath.Join(config.Roots[0], "agent"))
	f.Add("/etc", "/l/u", "/l/w", "/r/x")
	f.Add(filepath.Join(config.Templates[0], "out"), filepath.Join(config.Layers[0], "u"), filepath.Join(config.Layers[0], "w"), filepath.Join(config.Roots[0], "agent"))
	f.Add(config.Templates[0]+"/../../etc", config.Layers[0]+"/u", config.Layers[0]+"/w", config.Roots[0]+"/agent")
	beneath := func(base, path string) bool {
		relative, err := filepath.Rel(base, path)
		return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, "../") && !filepath.IsAbs(relative)
	}
	f.Fuzz(func(t *testing.T, lower, upper, work, target string) {
		layers, err := checkMount(config, config.UID, Request{Op: "mount", Lower: lower, Upper: upper, Work: work, Target: target})
		if err != nil {
			return
		}
		defer layers.close()
		for _, check := range []struct{ base, path string }{
			{config.Templates[0], lower}, {config.Layers[0], upper}, {config.Layers[0], work}, {config.Roots[0], target},
		} {
			if !beneath(check.base, check.path) {
				t.Fatalf("%q was accepted outside %s", check.path, check.base)
			}
			real, err := filepath.EvalSymlinks(check.path)
			if err != nil || real != check.path {
				t.Fatalf("%q was accepted through a symbolic link (%q)", check.path, real)
			}
		}
	})
}

func TestATemplateOutsideEveryTenantsIsNotPinned(t *testing.T) {
	templates := []string{"/var/lib/gitd/tenants/acme/templates", "/var/lib/gitd/tenants/globex/templates"}
	if _, err := pinUnderAny(templates, "templates", "/var/lib/gitd/tenants/other/templates/x", 1500); err == nil {
		t.Error("a template outside every tenant's was pinned")
	}
}
