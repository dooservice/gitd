package server

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// publishingHandlers answers PublishingService.
type publishingHandlers struct {
	server *Server
}

func (handlers *publishingHandlers) GetSettings(
	context.Context, *connect.Request[gitdv1.GetSettingsRequest],
) (*connect.Response[gitdv1.GetSettingsResponse], error) {
	return connect.NewResponse(&gitdv1.GetSettingsResponse{Settings: handlers.server.Publishing.All()}), nil
}

func (handlers *publishingHandlers) SetSettings(
	_ context.Context, request *connect.Request[gitdv1.SetSettingsRequest],
) (*connect.Response[gitdv1.SetSettingsResponse], error) {
	set, err := handlers.server.Publishing.Set(request.Msg.GetSettings())
	if err != nil {
		return nil, refuseChange(err)
	}
	handlers.server.recordChange(gitdv1.ConfigurationChange_ACTION_SETTINGS_SET, set.GetUrlPrefix(), set.GetUrlPrefix())
	return connect.NewResponse(&gitdv1.SetSettingsResponse{Settings: set}), nil
}

func (handlers *publishingHandlers) RemoveSettings(
	_ context.Context, request *connect.Request[gitdv1.RemoveSettingsRequest],
) (*connect.Response[gitdv1.RemoveSettingsResponse], error) {
	prefix := request.Msg.GetUrlPrefix()
	if err := handlers.server.Publishing.Remove(prefix); err != nil {
		return nil, refuseChange(err)
	}
	handlers.server.recordChange(gitdv1.ConfigurationChange_ACTION_SETTINGS_REMOVED, prefix, prefix)
	return connect.NewResponse(&gitdv1.RemoveSettingsResponse{}), nil
}

func (handlers *publishingHandlers) ListPresets(
	context.Context, *connect.Request[gitdv1.ListPresetsRequest],
) (*connect.Response[gitdv1.ListPresetsResponse], error) {
	return connect.NewResponse(&gitdv1.ListPresetsResponse{Presets: handlers.server.Publishing.Presets()}), nil
}

func (handlers *publishingHandlers) SetPreset(
	_ context.Context, request *connect.Request[gitdv1.SetPresetRequest],
) (*connect.Response[gitdv1.SetPresetResponse], error) {
	set, err := handlers.server.Publishing.SetPreset(request.Msg.GetPreset())
	if err != nil {
		return nil, refuseChange(err)
	}
	handlers.server.recordChange(gitdv1.ConfigurationChange_ACTION_PRESET_SET, set.GetName(), "")
	return connect.NewResponse(&gitdv1.SetPresetResponse{Preset: set}), nil
}

func (handlers *publishingHandlers) RemovePreset(
	_ context.Context, request *connect.Request[gitdv1.RemovePresetRequest],
) (*connect.Response[gitdv1.RemovePresetResponse], error) {
	name := request.Msg.GetName()
	if err := handlers.server.Publishing.RemovePreset(name); err != nil {
		return nil, refuseChange(err)
	}
	handlers.server.recordChange(gitdv1.ConfigurationChange_ACTION_PRESET_REMOVED, name, "")
	return connect.NewResponse(&gitdv1.RemovePresetResponse{}), nil
}
