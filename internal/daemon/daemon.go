// Package daemon is gitd serving: it reads the configuration, puts every part together
// and answers on the socket until it is told to stop.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/housekeeping"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/mounthelper"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/server"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
	"github.com/dooservice/gitd/internal/workspace"
)

// Run serves with the configuration at path until ctx ends. A socket, when given, wins
// over the configuration's.
func Run(ctx context.Context, path, socket, version string) error {
	settings, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if socket != "" {
		settings.Socket = socket
	}

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

	templates := template.New(settings.Templates(), locks, workspaces)
	want := strategy.Name(settings.Workspaces.Strategy)
	if want == "auto" {
		want = ""
	}
	maker, why := strategy.Detect(want, settings.State, settings.Roots, mounters(settings.Workspaces.MountHelper)...)
	workspaces.UseTemplates(templates, maker)
	made := strategy.Copy
	if maker != nil {
		made = maker.Name()
	}
	slog.Info("workspaces are made by "+string(made), "why", why)
	keeper := housekeeping.New(pools, templates, runner, settings.Roots, settings.Housekeeping, measured)
	keeper.Sweep()
	go keeper.Run(ctx)

	served := &server.Server{
		Version:        version,
		Workspaces:     workspaces,
		Pools:          pools,
		Runner:         runner,
		Metrics:        measured,
		Operation:      limits.Operation,
		Totals:         keeper.Totals,
		RetirePools:    keeper.Retire,
		Strategy:       made.Proto(),
		StrategyReason: why,
		TemplateTotals: keeper.TemplateTotals,
	}
	return served.Serve(ctx, settings.Socket, settings.Metrics)
}

// mounters are who may mount an overlay, in the order they are tried: the kernel itself,
// when gitd runs with the privilege to, then gitd-mount, when its socket is there.
func mounters(helper string) []strategy.Mounter {
	tried := []strategy.Mounter{strategy.Kernel{}}
	if helper == "" {
		helper = config.DefaultMountHelper
	}
	if _, err := os.Stat(helper); err == nil {
		tried = append(tried, mounthelper.Client{Socket: helper})
	}
	return tried
}
