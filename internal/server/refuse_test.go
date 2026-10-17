package server

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/metrics"
)

func TestEveryRefusalCarriesTheCodeOfWhatHappened(t *testing.T) {
	server := &Server{Metrics: metrics.New(func() float64 { return 0 })}
	for err, want := range map[error]connect.Code{
		git.Fail(git.NotFound, "no branch"):  connect.CodeNotFound,
		git.Fail(git.Busy, "queue full"):     connect.CodeResourceExhausted,
		git.Fail(git.AuthRefused, "refused"): connect.CodePermissionDenied,
		context.Canceled:                     connect.CodeCanceled,
		context.DeadlineExceeded:             connect.CodeDeadlineExceeded,
		errors.New("disk full"):              connect.CodeInternal,
	} {
		if got := connect.CodeOf(server.refuse(err)); got != want {
			t.Errorf("refuse(%v) = %v; want %v", err, got, want)
		}
	}
	refused := server.refuse(git.Fail(git.NotFound, "no branch"))
	var connectErr *connect.Error
	if !errors.As(refused, &connectErr) || connectErr.Meta().Get("Gitd-Reason") != git.NotFound.String() {
		t.Errorf("no Gitd-Reason on %v", refused)
	}
}
