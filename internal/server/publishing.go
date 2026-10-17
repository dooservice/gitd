package server

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/gen/go/gitd/v1/gitdv1connect"
)

// publishingHandlers answers PublishingService. Presets come with their own changes; until
// then those calls answer that they are not implemented.
type publishingHandlers struct {
	gitdv1connect.UnimplementedPublishingServiceHandler
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
