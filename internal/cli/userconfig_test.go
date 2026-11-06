package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/dooservice/gitd/internal/config"
)

func TestTheUsersConfigurationIsWrittenOnceAndReadsBack(t *testing.T) {
	dir := t.TempDir()
	places := config.User{
		Config: filepath.Join(dir, "config", "gitd", "gitd.toml"),
		State:  filepath.Join(dir, "state", "gitd"),
		Socket: filepath.Join(dir, "run", "gitd", "gitd.sock"),
		Root:   filepath.Join(dir, "home", "gitd", "workspaces"),
	}
	first := userConfig{places: places, git: filepath.Join(dir, "bin", "git"), gitHub: true}
	written, err := first.write()
	if err != nil || !written {
		t.Fatalf("write = %v, %v", written, err)
	}
	settings, err := config.Load(places.Config)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Socket != places.Socket || settings.State != places.State || !slices.Equal(settings.Roots, []string{places.Root}) ||
		settings.Git != first.git {
		t.Errorf("read back %q, %q, %q, %q", settings.Socket, settings.State, settings.Roots, settings.Git)
	}
	want := config.Credential{Prefix: "https://github.com", TokenEnv: gitHubTokenVariable}
	if len(settings.Credentials) != 1 || settings.Credentials[0].Prefix != want.Prefix || settings.Credentials[0].TokenEnv != want.TokenEnv {
		t.Errorf("credentials %+v; want only %+v", settings.Credentials, want)
	}
	info, err := os.Stat(places.Config)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v; want only the user to read it", info.Mode())
	}

	second := userConfig{places: places}
	if written, err := second.write(); err != nil || written {
		t.Fatalf("a second write = %v, %v; want the first kept", written, err)
	}
	if kept, _ := config.Load(places.Config); len(kept.Credentials) != 1 {
		t.Errorf("the first configuration was overwritten: %+v", kept)
	}
}

func TestWithoutTheGitHubCLIThereIsNoCredential(t *testing.T) {
	text, err := userConfig{places: config.User{Config: "c", State: "s", Socket: "k", Root: "r"}}.text()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), "credentials") || strings.Contains(string(text), "git =") {
		t.Errorf("without gh or git found, it says\n%s", text)
	}
}
