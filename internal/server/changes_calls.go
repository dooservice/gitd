package server

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func (server *Server) Changes(
	ctx context.Context, request *connect.Request[gitdv1.ChangesRequest],
) (*connect.Response[gitdv1.ChangesResponse], error) {
	answer, err := server.Workspaces.Changes(ctx, request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(answer), nil
}

func (server *Server) Diff(
	ctx context.Context, request *connect.Request[gitdv1.DiffRequest],
) (*connect.Response[gitdv1.DiffResponse], error) {
	answer, err := server.Workspaces.Diff(ctx, request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(answer), nil
}
