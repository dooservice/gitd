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
		UrlPrefix: "https://github.com/acme", Preset: "recommended",
	}})); err != nil {
		t.Fatal(err)
	}
	got, err := handlers.GetSettings(ctx, connect.NewRequest(&gitdv1.GetSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if settings := got.Msg.GetSettings(); len(settings) != 2 || !settings[0].GetFromFile() ||
		settings[1].GetPreset() != "recommended" {
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

func TestPresetsAreListedSetAndRemovedThroughTheAPIAndRecorded(t *testing.T) {
	server := connectionsServer(t)
	var err error
	server.Publishing, err = publishing.Open(t.TempDir(), publishing.File{})
	if err != nil {
		t.Fatal(err)
	}
	handlers := &publishingHandlers{server: server}
	ctx := context.Background()
	listed, err := handlers.ListPresets(ctx, connect.NewRequest(&gitdv1.ListPresetsRequest{}))
	if err != nil || len(listed.Msg.GetPresets()) != 1 || listed.Msg.GetPresets()[0].GetOrigin() != gitdv1.Preset_ORIGIN_BUILT_IN {
		t.Fatalf("listed = %v, %v", listed.Msg, err)
	}
	if _, err := handlers.SetPreset(ctx, connect.NewRequest(&gitdv1.SetPresetRequest{Preset: &gitdv1.Preset{
		Name: "strict", ProtectedPaths: []string{"deploy/**"},
	}})); err != nil {
		t.Fatal(err)
	}
	if _, err := handlers.SetSettings(ctx, connect.NewRequest(&gitdv1.SetSettingsRequest{Settings: &gitdv1.PublishSettings{
		Preset: "strict",
	}})); err != nil {
		t.Fatal(err)
	}
	var refused *connect.Error
	if _, err := handlers.RemovePreset(ctx, connect.NewRequest(&gitdv1.RemovePresetRequest{Name: "strict"})); !errors.As(err, &refused) ||
		refused.Code() != connect.CodeFailedPrecondition {
		t.Errorf("removing a preset in use: %v", err)
	}
	entries, err := server.AuditLog.Read(audit.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[1].GetChange().GetAction() != gitdv1.ConfigurationChange_ACTION_PRESET_SET {
		t.Errorf("audit = %v", entries)
	}
}
