package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnSSHHostIsReadAndOneThatCannotBeIsRefused(t *testing.T) {
	dir := t.TempDir()
	write := func(text string) string {
		path := filepath.Join(dir, "gitd.toml")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	settings, err := Load(write("[ssh.\"Git.Example.com\"]\nport = 2222\nuser = \"forgejo\"\n"))
	if err != nil || settings.SSH["git.example.com"] != (SSHHost{Port: 2222, User: "forgejo"}) {
		t.Fatalf("Load = %v, %v", settings.SSH, err)
	}
	for _, text := range []string{
		"[ssh.\"git.example.com\"]\nport = 70000\n",
		"[ssh.\"git.example.com\"]\nuser = \"-oProxyCommand=x\"\n",
		"[ssh.\"git.example.com\"]\nuser = \"a@b\"\n",
		"[ssh.\"host:22\"]\nport = 2222\n",
		"[ssh.\"host\tx\"]\nport = 2222\n",
		"[ssh.\"Git.example.com\"]\nport = 1\n[ssh.\"git.example.com\"]\nport = 2\n",
	} {
		if _, err := Load(write(text)); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}

func TestAMissingFileIsTheDefaults(t *testing.T) {
	settings, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil || settings.Socket != DefaultSocket || settings.State != Default().State {
		t.Fatalf("Load = %+v, %v", settings, err)
	}
}
