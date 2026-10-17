package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/repo"
)

func TestAHostIsReadAndOneThatCannotBeReachedIsRefused(t *testing.T) {
	dir := t.TempDir()
	write := func(text string) string {
		path := filepath.Join(dir, "gitd.toml")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	settings, err := Load(write("[hosts.\"Git.Example.com\"]\nhttps_port = 3000\nssh_port = 2222\nssh_user = \"forgejo\"\n"))
	if err != nil || settings.Hosts["git.example.com"] != (repo.Host{HTTPSPort: 3000, SSHPort: 2222, SSHUser: "forgejo"}) {
		t.Fatalf("Load = %v, %v", settings.Hosts, err)
	}
	for _, text := range []string{
		"[hosts.\"git.example.com\"]\nssh_port = 70000\n",
		"[hosts.\"git.example.com\"]\nssh_user = \"-oProxyCommand=x\"\n",
		"[hosts.\"git.example.com\"]\nssh_user = \"a@b\"\n",
		"[hosts.\"host:22\"]\nssh_port = 2222\n",
		"[hosts.\"host\tx\"]\nssh_port = 2222\n",
		"[hosts.\"Git.example.com\"]\nssh_port = 1\n[hosts.\"git.example.com\"]\nssh_port = 2\n",
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

func TestACredentialNamesExactlyOneSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitd.toml")
	for text, valid := range map[string]bool{
		"[[credentials]]\nprefix = \"https://github.com/acme\"\ntoken_env = \"T\"\n":                                      true,
		"[[credentials]]\nprefix = \"\"\nssh_key = \"/k\"\n":                                                              true,
		"[[credentials]]\nprefix = \"x\"\n":                                                                               false,
		"[[credentials]]\nprefix = \"x\"\ntoken_env = \"T\"\nssh_key = \"/k\"\n":                                          false,
		"[[credentials]]\nprefix = \"x\"\nssh_key = \"/k\"\ntoken_user = \"oauth2\"\n":                                    false,
		"[[credentials]]\nprefix = \"https://github.com/acme\"\ngithub_app = { app_id = 7, private_key_file = \"/k\" }\n": true,
		"[[credentials]]\nprefix = \"x\"\ngithub_app = { app_id = 7 }\n":                                                  false,
		"[[credentials]]\nprefix = \"x\"\ntoken_env = \"T\"\ngithub_app = { app_id = 7, private_key_file = \"/k\" }\n":    false,
	} {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); (err == nil) != valid {
			t.Errorf("Load(%q) = %v; valid %v", text, err, valid)
		}
	}
}

func TestAStrategyIsOneGitdKnows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gitd.toml")
	for strategy, valid := range map[string]bool{"": true, "auto": true, "overlay": true, "copy": true, "reflink": true, "fuse": false} {
		if err := os.WriteFile(path, []byte("[workspaces]\nstrategy = \""+strategy+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); (err == nil) != valid {
			t.Errorf("strategy %q: %v; valid %v", strategy, err, valid)
		}
	}
}

func TestABranchPrefixCanStartABranchName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gitd.toml")
	prefixes := map[string]bool{
		"": true, "agents/": true, "bot-": true, "team/agents/": true,
		"-x/": false, "/agents/": false, "a..b/": false, "a//b": false, "agents /": false, "agents~": false,
	}
	for prefix, valid := range prefixes {
		if err := os.WriteFile(path, []byte("[publish]\nbranch_prefix = \""+prefix+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); (err == nil) != valid {
			t.Errorf("branch_prefix %q: %v; valid %v", prefix, err, valid)
		}
	}
}

func TestPublishSettingsAndTheSecretsKeyAreRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gitd.toml")
	text := `secrets_key_file = "/run/credentials/gitd.service/secrets"

[[publish.presets]]
name = "strict"
protected_paths = ["deploy/**"]
max_changed_files = 20

[[publish.settings]]
prefix = "https://github.com/acme"
preset = "recommended"
protected_paths = ["deploy/**"]
allowed_paths = [".gitmodules"]
max_changed_files = 50
branch_prefix = "agents/"
pull_requests = { open = "always", draft = true, labels = ["agent"], request_review_from_requester = true }
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.SecretsKeyFile != "/run/credentials/gitd.service/secrets" {
		t.Errorf("secrets_key_file = %q", settings.SecretsKeyFile)
	}
	if presets := settings.Publish.Presets; len(presets) != 1 || presets[0].Name != "strict" || *presets[0].MaxChangedFiles != 20 {
		t.Errorf("presets = %+v", presets)
	}
	if len(settings.Publish.Settings) != 1 || settings.Publish.Settings[0].AllowedPaths[0] != ".gitmodules" {
		t.Fatalf("settings = %+v", settings.Publish.Settings)
	}
	setting := settings.Publish.Settings[0]
	if setting.Preset != "recommended" || setting.MaxChangedFiles == nil || *setting.MaxChangedFiles != 50 ||
		setting.PullRequests.Open != "always" || !setting.PullRequests.Draft || setting.PullRequests.Labels[0] != "agent" {
		t.Errorf("setting = %+v", setting)
	}
}
