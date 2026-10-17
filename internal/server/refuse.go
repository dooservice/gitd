package server

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"connectrpc.com/connect"

	"github.com/dooservice/gitd/internal/git"
)

// refuse turns an error into what the caller reads: a code, and what git said, both as a
// typed detail and as metadata. The metadata is what a client without protobuf reads: it
// travels as headers, or in the end of a stream.
func (server *Server) refuse(err error) error {
	failure, ok := git.AsFailure(err)
	switch {
	case ok:
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
	server.Metrics.Failures.WithLabelValues(failure.Reason.String()).Inc()
	refused := connect.NewError(codeOf(failure.Reason), failure)
	if detail, err := connect.NewErrorDetail(failure.Proto()); err == nil {
		refused.AddDetail(detail)
	}
	refused.Meta().Set("Gitd-Reason", failure.Reason.String())
	refused.Meta().Set("Gitd-Repository", failure.Repository)
	refused.Meta().Set("Gitd-Said", oneLine(failure.GitSaid))
	return refused
}

// oneLine is text that can travel as a header: no line breaks, no control characters.
func oneLine(text string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, text)
}

func codeOf(reason git.Reason) connect.Code {
	switch reason {
	case git.AuthRequired:
		return connect.CodeUnauthenticated
	case git.AuthRefused, git.PathRefused, git.BranchRefused:
		return connect.CodePermissionDenied
	case git.NotFound:
		return connect.CodeNotFound
	case git.BadAddress:
		return connect.CodeInvalidArgument
	case git.Busy:
		return connect.CodeResourceExhausted
	case git.Timeout:
		return connect.CodeDeadlineExceeded
	case git.Corrupt:
		return connect.CodeDataLoss
	case git.NothingNew, git.ConfigRefused:
		return connect.CodeFailedPrecondition
	default:
		return connect.CodeInternal
	}
}
