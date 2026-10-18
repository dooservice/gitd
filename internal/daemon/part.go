package daemon

import (
	"context"

	"github.com/dooservice/gitd/internal/audit"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/connections"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/endpoint"
	"github.com/dooservice/gitd/internal/events"
	"github.com/dooservice/gitd/internal/housekeeping"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/pullrequests"
	"github.com/dooservice/gitd/internal/secrets"
	"github.com/dooservice/gitd/internal/server"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
	"github.com/dooservice/gitd/internal/workspace"
)

// part is one part of gitd: everything one tenant has, or the implicit one, put together
// from its own configuration and nothing of another's.
type part struct {
	*server.Tenant
	workspaces *workspace.Workspaces
	keeper     *housekeeping.Keeper
	endpoint   *server.Endpoint
}

// part puts together the part of gitd that settings describe — a tenant's when tenant is
// named — starts its maintenance, and answers what serves it.
func (shared shared) part(ctx context.Context, settings config.Config, tenant string) (*part, error) {
	log := logged(tenant)
	runner := shared.runner
	stored, err := credentials.Stored(settings.Credentials)
	if err != nil {
		return nil, err
	}
	keys, err := credentials.NewKeys(settings.State, settings.KnownHosts, settings.Hosts, stored)
	if err != nil {
		return nil, err
	}
	box, err := secrets.Open(settings.SecretsKeyFile)
	if err != nil {
		return nil, err
	}
	kept, err := connections.Open(settings.Connections(), box)
	if err != nil {
		return nil, err
	}
	github := connections.GitHub{
		UserAgent: "gitd/" + shared.version, Hosts: settings.Hosts,
		LoginClientID: settings.GitHub.LoginClientID, LoginAppSlug: settings.GitHub.LoginApp,
	}
	kept.UseGitHub(github)
	fileApps, err := connections.NewFileApps(settings.Credentials, github)
	if err != nil {
		return nil, err
	}
	keys.AlsoKeep(fileApps)
	keys.AlsoKeep(kept)
	fileSettings, err := publishing.FromFile(settings.Publish)
	if err != nil {
		return nil, err
	}
	publishingSettings, err := publishing.Open(settings.PublishingSettings(), fileSettings)
	if err != nil {
		return nil, err
	}
	locks, err := lock.NewLocks(settings.Locks(), settings.Limits.QueueWait)
	if err != nil {
		return nil, err
	}
	limits := settings.Limits
	fetches := lock.NewQueue(limits.Fetches, limits.Queued, limits.QueueWait)
	writes := lock.NewQueue(limits.Writes, limits.Queued, limits.QueueWait)

	pools := pool.New(settings.Pools(), runner, keys, locks, fetches, settings.Housekeeping.StaleLock)
	workspaces := workspace.New(settings.Roots, runner, pools, locks, writes, settings.Housekeeping.StaleLock)
	auditLog, err := audit.New(settings.AuditLog(), settings.Audit.RotateBytes, settings.Audit.Keep)
	if err != nil {
		return nil, err
	}
	bus := events.NewBus()
	workspaces.UseEvents(bus)
	workspaces.UsePublishing(workspace.Publishing{
		Settings: publishingSettings, PullRequests: pullrequests.New(keys, github), Recorder: auditLog,
	})

	templates := template.New(settings.Templates(), locks, workspaces)
	want := strategy.Name(settings.Workspaces.Strategy)
	if want == "auto" {
		want = ""
	}
	maker, why := strategy.Detect(want, settings.State, settings.Roots, mounters(settings.Workspaces.MountHelper)...)
	workspaces.UseTemplates(templates, maker)
	workspaces.RecordOwners(ctx)
	made := strategy.Copy
	if maker != nil {
		made = maker.Name()
	}
	log.Info("workspaces are made by "+string(made), "why", why)
	overlay, _ := maker.(*strategy.OverlayMaker)
	keeper := housekeeping.New(pools, templates, overlay, runner, settings.Roots, settings.Housekeeping, shared.metrics)
	keeper.Sweep()
	keeper.Remount(ctx)
	go keeper.Run(ctx)
	go workspaces.WatchLayers(ctx)

	served := &server.Server{
		Version:        shared.version,
		Workspaces:     workspaces,
		Pools:          pools,
		Runner:         runner,
		Metrics:        shared.metrics,
		Operation:      limits.Operation,
		Totals:         keeper.Totals,
		RetirePools:    keeper.Retire,
		Strategy:       made.Proto(),
		StrategyReason: why,
		TemplateTotals: keeper.TemplateTotals,
		AuditLog:       auditLog,

		Connections:     kept,
		FileConnections: connections.ShownFromFile(settings.Credentials),
		Publishing:      publishingSettings,
		Events:          bus,
	}
	built := &part{Tenant: &server.Tenant{Name: tenant}, workspaces: workspaces, keeper: keeper}
	if settings.Endpoint.Listen != "" {
		tokens := endpoint.NewTokens()
		sandboxes := endpoint.NewSandboxes(settings.Sandboxes(), runner, pools)
		go sandboxes.RetireEvery(ctx, settings.Housekeeping.Every, settings.Housekeeping.RetireAfter, tokens)
		built.endpoint = &server.Endpoint{Tokens: tokens, Sandboxes: sandboxes, URL: settings.Endpoint.URL}
		served.Endpoint = built.endpoint
		built.Git = endpoint.NewHandler(runner, tokens, sandboxes, settings.Endpoint.MaxPushBytes)
	}
	built.API = served.Routes()
	return built, nil
}

// limit holds a tenant's part to the tenant's limits.
func (built *part) limit(tenant config.Tenant) error {
	disk, err := tenant.Limits.DiskBytes()
	if err != nil {
		return err
	}
	built.workspaces.UseLimits(workspace.Limits{Workspaces: tenant.Limits.Workspaces, Disk: disk, Used: built.keeper.DiskUsed})
	if built.endpoint != nil {
		built.endpoint.Limits = built.workspaces
	}
	return nil
}
