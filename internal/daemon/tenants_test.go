package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/client"
)

// TestTwoTenantsOnOneGitdSeeNothingOfEachOther runs gitd with two tenants and tries, with
// each one's key, to reach the other's: its workspaces, its publishing settings, its audit
// log. Every try is refused; a call with no key, or a key gitd does not know, is too.
func TestTwoTenantsOnOneGitdSeeNothingOfEachOther(t *testing.T) {
	root, err := os.MkdirTemp("", "gitd-tenants-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	socket := filepath.Join(root, "gitd.sock")
	keys := map[string]string{"acme": "acme-key-0123456789", "globex": "globex-key-0123456789"}
	text := fmt.Sprintf("socket = %q\nstate = %q\nroots = []\n", socket, filepath.Join(root, "state"))
	for name, key := range keys {
		keyFile := filepath.Join(root, name+".key")
		if err := os.WriteFile(keyFile, []byte(key+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		text += fmt.Sprintf("[[tenants]]\nname = %q\napi_key_file = %q\nroots = [%q]\n", name, keyFile, filepath.Join(root, "roots", name))
	}
	configPath := filepath.Join(root, "gitd.toml")
	if err := os.WriteFile(configPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Run(ctx, configPath, "", "test") }()
	t.Cleanup(func() { cancel(); <-served })
	for range 100 {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	as := func(key string) *client.Client {
		called, err := client.Open(client.Options{Socket: socket, Key: key})
		if err != nil {
			t.Fatal(err)
		}
		return called
	}
	acme, globex := as(keys["acme"]), as(keys["globex"])
	for name, caller := range map[string]*client.Client{"no key": as(""), "a key gitd does not know": as("made-up")} {
		if _, err := caller.Info(ctx); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s: %v; want unauthenticated", name, err)
		}
	}
	if _, err := acme.Info(ctx); err != nil {
		t.Fatalf("acme's own key was refused: %v", err)
	}

	// globex's workspace path is outside acme's roots.
	_, err = acme.Describe(ctx, filepath.Join(root, "roots", "globex", "agent-1"))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("acme described a path of globex's: %v", err)
	}

	// Publishing settings and the audit log are each tenant's own.
	if _, err := acme.Publishing.SetSettings(ctx, connect.NewRequest(&gitdv1.SetSettingsRequest{
		Settings: &gitdv1.PublishSettings{UrlPrefix: "https://github.com/acme", BranchPrefix: "agents/"},
	})); err != nil {
		t.Fatal(err)
	}
	theirs, err := globex.Publishing.GetSettings(ctx, connect.NewRequest(&gitdv1.GetSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range theirs.Msg.GetSettings() {
		if setting.GetUrlPrefix() == "https://github.com/acme" {
			t.Error("globex sees acme's publishing settings")
		}
	}
	recorded, err := globex.Audit(ctx, &gitdv1.AuditRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded.GetEntries()) != 0 {
		t.Errorf("globex's audit log holds %v", recorded.GetEntries())
	}
	ours, err := acme.Audit(ctx, &gitdv1.AuditRequest{})
	if err != nil || len(ours.GetEntries()) != 1 {
		t.Errorf("acme's audit log = %v, %v; want its settings change", ours.GetEntries(), err)
	}
	for _, name := range []string{"acme", "globex"} {
		if _, err := os.Stat(filepath.Join(root, "state", "tenants", name, "audit")); err != nil {
			t.Errorf("%s has no state of its own: %v", name, err)
		}
	}
}
