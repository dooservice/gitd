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
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/housekeeping"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/server"
	"github.com/dooservice/gitd/internal/workspace"
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
	if err := run(*path, *socket); err != nil {
		slog.Error("gitd stopped", "error", err)
		os.Exit(1)
	}
}

func run(path, socket string) error {
	settings, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if socket != "" {
		settings.Socket = socket
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runner := git.NewRunner(settings.Git)
	if _, err := runner.Version(ctx); err != nil {
		return fmt.Errorf("running %s: %w", settings.Git, err)
	}
	stored, err := credentials.Stored(settings.Credentials)
	if err != nil {
		return err
	}
	keys, err := credentials.NewKeys(settings.State, settings.KnownHosts, settings.Hosts, stored)
	if err != nil {
		return err
	}
	locks, err := lock.NewLocks(settings.Locks(), settings.Limits.QueueWait)
	if err != nil {
		return err
	}
	limits := settings.Limits
	fetches := lock.NewQueue(limits.Fetches, limits.Queued, limits.QueueWait)
	writes := lock.NewQueue(limits.Writes, limits.Queued, limits.QueueWait)
	measured := metrics.New(func() float64 { return float64(runner.Running()) })

	pools := pool.New(settings.Pools(), runner, keys, locks, fetches, settings.Housekeeping.StaleLock)
	workspaces := workspace.New(settings.Roots, runner, pools, locks, writes, settings.Housekeeping.StaleLock)

	keeper := housekeeping.New(pools, runner, settings.Roots, settings.Housekeeping, measured)
	keeper.Sweep()
	go keeper.Run(ctx)

	served := &server.Server{
		Version:     version,
		Workspaces:  workspaces,
		Pools:       pools,
		Runner:      runner,
		Metrics:     measured,
		Operation:   limits.Operation,
		Totals:      keeper.Totals,
		RetirePools: keeper.Retire,
	}
	return served.Serve(ctx, settings.Socket, settings.Metrics)
}
