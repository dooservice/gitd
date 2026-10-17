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

func (server *Server) Compare(
	ctx context.Context, request *connect.Request[gitdv1.CompareRequest],
) (*connect.Response[gitdv1.CompareResponse], error) {
	answer, err := server.Workspaces.Compare(ctx, request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(answer), nil
}

func (server *Server) Conflicts(
	ctx context.Context, request *connect.Request[gitdv1.ConflictsRequest],
) (*connect.Response[gitdv1.ConflictsResponse], error) {
	answer, err := server.Workspaces.Conflicts(ctx, request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(answer), nil
}

// Watch streams what happens to the workspaces asked for, or to every one, until the
// caller goes or falls too far behind.
func (server *Server) Watch(
	ctx context.Context, request *connect.Request[gitdv1.WatchRequest], stream *connect.ServerStream[gitdv1.WatchResponse],
) error {
	if server.Events == nil {
		return connect.NewError(connect.CodeUnimplemented, errors.New("this gitd tells no events"))
	}
	watcher := server.Events.Watch(request.Msg.GetPaths())
	defer server.Events.Stop(watcher)
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, open := <-watcher.Events:
			if !open {
				return connect.NewError(connect.CodeResourceExhausted, errors.New("the watch fell too far behind"))
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}
