// Command gitd keeps the copies of git repositories on one server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/daemon"
)

var version = "dev"

func main() {
	path := flag.String("config", config.DefaultPath, "configuration file")
	socket := flag.String("socket", "", "Unix socket to answer on, over the configuration's")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := daemon.Run(ctx, *path, *socket, version); err != nil {
		slog.Error("gitd stopped", "error", err)
		os.Exit(1)
	}
}
