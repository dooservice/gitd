// Package daemon is gitd serving: it reads the configuration, puts every part together
// and answers on the socket until it is told to stop.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/dooservice/gitd/internal/apikeys"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/mounthelper"
	"github.com/dooservice/gitd/internal/server"
	"github.com/dooservice/gitd/internal/strategy"
)

// Run serves with the configuration at path until ctx ends. A socket, when given, wins
// over the configuration's.
//
// Without tenants gitd is one part, as it always was. With tenants, each is a part of its
// own — pools, templates, workspaces, credentials, tokens and audit log under its own
// directory — and the gateway sends every call to the part its key names.
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
	measured := metrics.New(func() float64 { return float64(runner.Running()) })
	gateway := &server.Gateway{
		Version: version, Socket: settings.Socket, MetricsAddress: settings.Metrics, Metrics: measured,
	}
	if listen := settings.Endpoint; listen.Listen != "" {
		gateway.Network = &server.Network{Listen: listen.Listen, TLSCertFile: listen.TLSCertFile, TLSKeyFile: listen.TLSKeyFile}
	}
	shared := shared{runner: runner, metrics: measured, version: version}
	if !settings.HasTenants() {
		implicit, err := shared.part(ctx, settings, "")
		if err != nil {
			return err
		}
		if settings.APIKeyFile != "" {
			implicit.Key = apikeys.NewFile(settings.APIKeyFile)
		}
		gateway.Implicit = implicit.Tenant
		return gateway.Serve(ctx)
	}
	for _, tenant := range settings.Tenants {
		part, err := shared.part(ctx, settings.ForTenant(tenant), tenant.Name)
		if err != nil {
			return fmt.Errorf("tenant %s: %w", tenant.Name, err)
		}
		part.Key = apikeys.NewFile(tenant.APIKeyFile)
		if err := part.limit(tenant); err != nil {
			return fmt.Errorf("tenant %s: %w", tenant.Name, err)
		}
		gateway.Tenants = append(gateway.Tenants, part.Tenant)
	}
	return gateway.Serve(ctx)
}

// shared is what every part of gitd uses alike: the git it runs and the metrics it keeps.
type shared struct {
	runner  *git.Runner
	metrics *metrics.Metrics
	version string
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

// logged is the logger of a tenant's part, or gitd's own for the implicit one.
func logged(tenant string) *slog.Logger {
	if tenant == "" {
		return slog.Default()
	}
	return slog.Default().With("tenant", tenant)
}
