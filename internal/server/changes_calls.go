package server

import (
	"context"
	"errors"

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

// Compare, Conflicts and Watch come with their own changes; until then they answer that
// they are not implemented.
func (server *Server) Compare(
	context.Context, *connect.Request[gitdv1.CompareRequest],
) (*connect.Response[gitdv1.CompareResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("comparing is not done yet"))
}

func (server *Server) Conflicts(
	context.Context, *connect.Request[gitdv1.ConflictsRequest],
) (*connect.Response[gitdv1.ConflictsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("conflicts are not read yet"))
}

func (server *Server) Watch(
	context.Context, *connect.Request[gitdv1.WatchRequest], *connect.ServerStream[gitdv1.WatchResponse],
) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New("watching is not done yet"))
}
