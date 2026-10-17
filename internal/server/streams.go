package server

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func (server *Server) Prepare(
	ctx context.Context,
	request *connect.Request[gitdv1.PrepareRequest],
	stream *connect.ServerStream[gitdv1.PrepareResponse],
) error {
	result, err := server.Workspaces.Prepare(ctx, request.Msg, func(progress *gitdv1.Progress) {
		_ = stream.Send(&gitdv1.PrepareResponse{Event: &gitdv1.PrepareResponse_Progress{Progress: progress}})
	})
	if err != nil {
		return server.refuse(err)
	}
	result.LFS = server.Workspaces.LFS(request.Msg.GetPath())
	return stream.Send(&gitdv1.PrepareResponse{Event: &gitdv1.PrepareResponse_Result{Result: result.Proto()}})
}

func (server *Server) Update(
	ctx context.Context,
	request *connect.Request[gitdv1.UpdateRequest],
	stream *connect.ServerStream[gitdv1.UpdateResponse],
) error {
	result, err := server.Workspaces.Update(ctx, request.Msg, func(progress *gitdv1.Progress) {
		_ = stream.Send(&gitdv1.UpdateResponse{Event: &gitdv1.UpdateResponse_Progress{Progress: progress}})
	})
	if err != nil {
		return server.refuse(err)
	}
	result.LFS = server.Workspaces.LFS(request.Msg.GetPath())
	return stream.Send(&gitdv1.UpdateResponse{Event: &gitdv1.UpdateResponse_Result{Result: result.Proto()}})
}

func (server *Server) Checkout(
	ctx context.Context,
	request *connect.Request[gitdv1.CheckoutRequest],
	stream *connect.ServerStream[gitdv1.CheckoutResponse],
) error {
	result, err := server.Workspaces.Checkout(ctx, request.Msg, func(progress *gitdv1.Progress) {
		_ = stream.Send(&gitdv1.CheckoutResponse{Event: &gitdv1.CheckoutResponse_Progress{Progress: progress}})
	})
	if err != nil {
		return server.refuse(err)
	}
	result.LFS = server.Workspaces.LFS(request.Msg.GetPath())
	return stream.Send(&gitdv1.CheckoutResponse{Event: &gitdv1.CheckoutResponse_Result{Result: result.Proto()}})
}

func (server *Server) SyncSubmodules(
	ctx context.Context,
	request *connect.Request[gitdv1.SyncSubmodulesRequest],
	stream *connect.ServerStream[gitdv1.SyncSubmodulesResponse],
) error {
	result, err := server.Workspaces.SyncSubmodules(ctx, request.Msg, func(progress *gitdv1.Progress) {
		_ = stream.Send(&gitdv1.SyncSubmodulesResponse{Event: &gitdv1.SyncSubmodulesResponse_Progress{Progress: progress}})
	})
	if err != nil {
		return server.refuse(err)
	}
	return stream.Send(&gitdv1.SyncSubmodulesResponse{Event: &gitdv1.SyncSubmodulesResponse_Result{Result: result}})
}

func (server *Server) Adopt(
	ctx context.Context, request *connect.Request[gitdv1.AdoptRequest], stream *connect.ServerStream[gitdv1.AdoptResponse],
) error {
	released, err := server.Workspaces.Adopt(ctx, request.Msg, func(progress *gitdv1.Progress) {
		_ = stream.Send(&gitdv1.AdoptResponse{Event: &gitdv1.AdoptResponse_Progress{Progress: progress}})
	})
	if err != nil {
		return server.refuse(err)
	}
	result := &gitdv1.AdoptResult{BytesReleased: released}
	return stream.Send(&gitdv1.AdoptResponse{Event: &gitdv1.AdoptResponse_Result{Result: result}})
}
