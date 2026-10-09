package server

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"github.com/dooservice/gitd/internal/metrics"
)

// callObserver gives every call a deadline, and logs and measures it once it ends.
type callObserver struct {
	metrics   *metrics.Metrics
	operation time.Duration
}

func (observer *callObserver) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, cancel := observer.bounded(ctx)
		defer cancel()
		started := time.Now()
		response, err := next(ctx, request)
		observer.ended(request.Spec().Procedure, started, err)
		return response, err
	}
}

func (observer *callObserver) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (observer *callObserver) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, cancel := observer.bounded(ctx)
		defer cancel()
		started := time.Now()
		err := next(ctx, conn)
		observer.ended(conn.Spec().Procedure, started, err)
		return err
	}
}

func (observer *callObserver) bounded(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, set := ctx.Deadline(); set {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, observer.operation)
}

func (observer *callObserver) ended(procedure string, started time.Time, err error) {
	took := time.Since(started)
	code := "ok"
	if err != nil {
		code = connect.CodeOf(err).String()
	}
	observer.metrics.Operations.WithLabelValues(procedure, code).Observe(took.Seconds())
	if err != nil {
		slog.Warn("call failed", "procedure", procedure, "took", took.Round(time.Millisecond), "code", code, "error", err)
		return
	}
	slog.Info("call", "procedure", procedure, "took", took.Round(time.Millisecond))
}
