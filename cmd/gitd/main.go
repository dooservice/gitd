// Command gitd keeps one copy of each git repository and makes workspaces that borrow from
// it. `gitd` alone serves; `gitd prepare …` and the other commands call the gitd that
// serves (internal/cli).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dooservice/gitd/internal/cli"
)

var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := cli.Execute(ctx, os.Args[1:], version)
	stop()
	os.Exit(code)
}
