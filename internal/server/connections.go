package server

import (
	"context"
	"slices"
	"strings"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/gen/go/gitd/v1/gitdv1connect"
	"github.com/dooservice/gitd/internal/connections"
)

// connectionHandlers answers ConnectionService. Connecting GitHub comes with its own
// changes; until then those calls answer that they are not implemented.
type connectionHandlers struct {
	gitdv1connect.UnimplementedConnectionServiceHandler
	server *Server
}

func (handlers *connectionHandlers) AddToken(
	_ context.Context, request *connect.Request[gitdv1.AddTokenRequest],
) (*connect.Response[gitdv1.AddTokenResponse], error) {
	record, err := connections.NewToken(request.Msg.GetUrlPrefix(), request.Msg.GetToken(), request.Msg.GetUser())
	if err != nil {
		return nil, refuseChange(err)
	}
	added, err := handlers.server.Connections.Add(record, []byte(request.Msg.GetToken()))
	if err != nil {
		return nil, refuseChange(err)
	}
	handlers.server.recordChange(gitdv1.ConfigurationChange_ACTION_CONNECTION_ADDED, added.ID, added.URLPrefix)
	return connect.NewResponse(&gitdv1.AddTokenResponse{Connection: added.Shown()}), nil
}

func (handlers *connectionHandlers) ListConnections(
	context.Context, *connect.Request[gitdv1.ListConnectionsRequest],
) (*connect.Response[gitdv1.ListConnectionsResponse], error) {
	listed := slices.Clone(handlers.server.FileConnections)
	for _, record := range handlers.server.Connections.List() {
		listed = append(listed, record.Shown())
	}
	return connect.NewResponse(&gitdv1.ListConnectionsResponse{Connections: listed}), nil
}

func (handlers *connectionHandlers) RemoveConnection(
	_ context.Context, request *connect.Request[gitdv1.RemoveConnectionRequest],
) (*connect.Response[gitdv1.RemoveConnectionResponse], error) {
	id := request.Msg.GetId()
	if strings.HasPrefix(id, connections.FilePrefix) {
		return nil, refuseChange(connections.ErrFromFile)
	}
	record, _, err := handlers.server.Connections.Get(id)
	if err != nil {
		return nil, refuseChange(err)
	}
	if err := handlers.server.Connections.Remove(id); err != nil {
		return nil, refuseChange(err)
	}
	handlers.server.recordChange(gitdv1.ConfigurationChange_ACTION_CONNECTION_REMOVED, id, record.URLPrefix)
	return connect.NewResponse(&gitdv1.RemoveConnectionResponse{}), nil
}

func (handlers *connectionHandlers) AddGitHubApp(
	ctx context.Context, request *connect.Request[gitdv1.AddGitHubAppRequest],
) (*connect.Response[gitdv1.AddGitHubAppResponse], error) {
	message := request.Msg
	added, err := handlers.server.Connections.AddApp(ctx, message.GetHost(), message.GetAppId(),
		[]byte(message.GetPrivateKey()), message.GetUrlPrefix())
	if err != nil {
		return nil, refuseChange(err)
	}
	handlers.server.recordChange(gitdv1.ConfigurationChange_ACTION_CONNECTION_ADDED, added.ID, added.URLPrefix)
	return connect.NewResponse(&gitdv1.AddGitHubAppResponse{Connection: added.Shown()}), nil
}
