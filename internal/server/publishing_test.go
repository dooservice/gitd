package server

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/audit"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/publishing"
)

func TestSettingsAreSetAndRemovedThroughTheAPIAndRecorded(t *testing.T) {
	server := connectionsServer(t)
	fromFile, err := publishing.FromFile(config.Publish{BranchPrefix: "agents/"})
	if err != nil {
		t.Fatal(err)
	}
	server.Publishing, err = publishing.Open(t.TempDir(), fromFile)
	if err != nil {
		t.Fatal(err)
	}
	handlers := &publishingHandlers{server: server}
	ctx := context.Background()
	if _, err := handlers.SetSettings(ctx, connect.NewRequest(&gitdv1.SetSettingsRequest{Settings: &gitdv1.PublishSettings{
		UrlPrefix: "https://github.com/acme", Preset: gitdv1.Preset_PRESET_RECOMMENDED,
	}})); err != nil {
		t.Fatal(err)
	}
	got, err := handlers.GetSettings(ctx, connect.NewRequest(&gitdv1.GetSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if settings := got.Msg.GetSettings(); len(settings) != 2 || !settings[0].GetFromFile() ||
		settings[1].GetPreset() != gitdv1.Preset_PRESET_RECOMMENDED {
		t.Fatalf("settings = %v", settings)
	}
	var refused *connect.Error
	if _, err := handlers.RemoveSettings(ctx, connect.NewRequest(&gitdv1.RemoveSettingsRequest{})); !errors.As(err, &refused) ||
		refused.Code() != connect.CodeFailedPrecondition {
		t.Errorf("removing the file's settings: %v", err)
	}
	if _, err := handlers.RemoveSettings(ctx, connect.NewRequest(&gitdv1.RemoveSettingsRequest{
		UrlPrefix: "https://github.com/acme",
	})); err != nil {
		t.Fatal(err)
	}
	entries, err := server.AuditLog.Read(audit.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].GetChange().GetAction() != gitdv1.ConfigurationChange_ACTION_SETTINGS_REMOVED {
		t.Errorf("audit = %v", entries)
	}
}
