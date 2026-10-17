package server

import (
	"errors"
	"log/slog"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/connections"
	"github.com/dooservice/gitd/internal/publishing"
)

// refuseChange turns why a change to connections or settings was refused into what the
// caller reads.
func refuseChange(err error) error {
	switch {
	case errors.Is(err, connections.ErrInvalid), errors.Is(err, publishing.ErrInvalid):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, connections.ErrFromFile), errors.Is(err, publishing.ErrFromFile),
		errors.Is(err, connections.ErrUnavailable), errors.Is(err, publishing.ErrInUse):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, connections.ErrNotFound), errors.Is(err, publishing.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

// recordChange keeps a change to connections or settings in the audit log. The change is
// made by the time it is recorded, so an entry that cannot be written is logged as an
// error, not answered as one.
func (server *Server) recordChange(action gitdv1.ConfigurationChange_Action, subject, prefix string) {
	if server.AuditLog == nil {
		return
	}
	entry := &gitdv1.AuditEntry{Change: &gitdv1.ConfigurationChange{Action: action, Subject: subject, UrlPrefix: prefix}}
	if err := server.AuditLog.Record(entry); err != nil {
		slog.Error("a change is missing from the audit log", "action", action.String(), "subject", subject, "error", err)
	}
}
