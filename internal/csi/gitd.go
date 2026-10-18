package csi

import (
	"context"
	"fmt"
	"os"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/client"
)

// gitdWorkspaces reaches one tenant's part of the gitd on the node, on its socket.
type gitdWorkspaces struct {
	gitd *client.Client
}

func (reached gitdWorkspaces) Prepare(ctx context.Context, request *gitdv1.PrepareRequest) (*gitdv1.WorkspaceResult, error) {
	return reached.gitd.Prepare(ctx, request, nil)
}

func (reached gitdWorkspaces) Remove(ctx context.Context, path string) error {
	return reached.gitd.Remove(ctx, path)
}

// TenantsOf reads which tenants the plugin serves: the implicit one at root, when gitd has
// none, and each `name=root,keyfile` given, reached on gitd's socket with that key.
func TenantsOf(socket, root string, given []string) (map[string]Tenant, error) {
	tenants := map[string]Tenant{}
	if len(given) == 0 {
		gitd, err := client.Open(client.Options{Socket: socket})
		if err != nil {
			return nil, err
		}
		tenants[""] = Tenant{Root: root, Workspaces: gitdWorkspaces{gitd: gitd}}
		return tenants, nil
	}
	for _, written := range given {
		name, rest, ok := strings.Cut(written, "=")
		tenantRoot, keyFile, both := strings.Cut(rest, ",")
		if !ok || !both || !kubernetesName.MatchString(name) {
			return nil, fmt.Errorf("--tenant %q: want name=root,keyfile", written)
		}
		key, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("tenant %s: %w", name, err)
		}
		gitd, err := client.Open(client.Options{Socket: socket, Key: strings.TrimSpace(string(key))})
		if err != nil {
			return nil, err
		}
		tenants[name] = Tenant{Root: tenantRoot, Workspaces: gitdWorkspaces{gitd: gitd}}
	}
	return tenants, nil
}

// Ready says whether every tenant's gitd answers.
func Ready(tenants map[string]Tenant) func(context.Context) error {
	return func(ctx context.Context) error {
		for _, tenant := range tenants {
			if reached, ok := tenant.Workspaces.(gitdWorkspaces); ok {
				if _, err := reached.gitd.Info(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}
}
