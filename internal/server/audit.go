package server

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/audit"
)

// How many audit entries a read returns when it does not say, and at most.
const (
	auditLimit    = 100
	auditLimitMax = 1000
)

func (server *Server) Audit(
	_ context.Context, request *connect.Request[gitdv1.AuditRequest],
) (*connect.Response[gitdv1.AuditResponse], error) {
	filter := audit.Filter{URL: request.Msg.GetUrl(), Limit: int(request.Msg.GetLimit())}
	if filter.Limit <= 0 {
		filter.Limit = auditLimit
	}
	filter.Limit = min(filter.Limit, auditLimitMax)
	if since := request.Msg.GetSince(); since != nil {
		filter.Since = since.AsTime()
	}
	entries, err := server.AuditLog.Read(filter)
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(&gitdv1.AuditResponse{Entries: entries}), nil
}
