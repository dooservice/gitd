package server

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
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

func TestInfoSaysHowWorkspacesAreMadeAndWhy(t *testing.T) {
	server := &Server{
		Version: "test", Runner: git.NewRunner("git"),
		Totals:         func() (int, int64) { return 2, 2048 },
		Strategy:       gitdv1.Strategy_STRATEGY_OVERLAY,
		StrategyReason: "an overlay mounts here",
		TemplateTotals: func() (int, int64) { return 3, 4096 },
	}
	answer, err := server.Info(context.Background(), connect.NewRequest(&gitdv1.InfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	info := answer.Msg
	if info.GetStrategy() != gitdv1.Strategy_STRATEGY_OVERLAY || info.GetStrategyReason() == "" || info.GetTemplates() != 3 || info.GetTemplatesBytes() != 4096 || info.GetPools() != 2 {
		t.Errorf("info = %+v", info)
	}
}
