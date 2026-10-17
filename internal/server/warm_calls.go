package server

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// Seal makes an overlay workspace's own layer a warm layer of its repository.
func (server *Server) Seal(
	ctx context.Context, request *connect.Request[gitdv1.SealRequest],
) (*connect.Response[gitdv1.SealResponse], error) {
	warm, err := server.Workspaces.Seal(ctx, request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(&gitdv1.SealResponse{Warm: warm}), nil
}

// ListWarm lists the warm layers of a repository.
func (server *Server) ListWarm(
	_ context.Context, request *connect.Request[gitdv1.ListWarmRequest],
) (*connect.Response[gitdv1.ListWarmResponse], error) {
	answer, err := server.Workspaces.ListWarm(request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(answer), nil
}
