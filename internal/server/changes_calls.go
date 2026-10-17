package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// Changes and Diff come with their own changes; until then they answer that they are not
// implemented.
func (server *Server) Changes(
	context.Context, *connect.Request[gitdv1.ChangesRequest],
) (*connect.Response[gitdv1.ChangesResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("changes are not read yet"))
}

func (server *Server) Diff(
	context.Context, *connect.Request[gitdv1.DiffRequest],
) (*connect.Response[gitdv1.DiffResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("diffs are not written yet"))
}
