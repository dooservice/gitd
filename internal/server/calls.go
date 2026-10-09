package server

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func (server *Server) Describe(
	ctx context.Context, request *connect.Request[gitdv1.DescribeRequest],
) (*connect.Response[gitdv1.DescribeResponse], error) {
	described, err := server.Workspaces.Describe(ctx, request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(described), nil
}

func (server *Server) Remove(
	ctx context.Context, request *connect.Request[gitdv1.RemoveRequest],
) (*connect.Response[gitdv1.RemoveResponse], error) {
	if err := server.Workspaces.Remove(ctx, request.Msg); err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(&gitdv1.RemoveResponse{}), nil
}

func (server *Server) Probe(
	ctx context.Context, request *connect.Request[gitdv1.ProbeRequest],
) (*connect.Response[gitdv1.ProbeResponse], error) {
	return connect.NewResponse(server.Pools.Probe(ctx, request.Msg)), nil
}

func (server *Server) Info(
	ctx context.Context, _ *connect.Request[gitdv1.InfoRequest],
) (*connect.Response[gitdv1.InfoResponse], error) {
	gitVersion, err := server.Runner.Version(ctx)
	if err != nil {
		return nil, server.refuse(err)
	}
	pools, poolsBytes := server.Totals()
	return connect.NewResponse(&gitdv1.InfoResponse{
		Version:    server.Version,
		GitVersion: gitVersion,
		Pools:      int32(pools),
		PoolsBytes: poolsBytes,
	}), nil
}

func (server *Server) Publish(
	ctx context.Context, request *connect.Request[gitdv1.PublishRequest],
) (*connect.Response[gitdv1.PublishResponse], error) {
	published, err := server.Workspaces.Publish(ctx, request.Msg)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(published), nil
}
