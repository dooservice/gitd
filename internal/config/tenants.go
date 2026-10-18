package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// tenantName is what a tenant may be called: a word a directory and a log line can carry.
var tenantName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Tenant is one customer of a platform that hosts many on one gitd. Each has its own key
// the platform calls with, its own roots, credentials and limits, and its own pools,
// templates, workspaces, tokens and audit log, under `<state>/tenants/<name>`: nothing is
// shared between tenants.
type Tenant struct {
	Name        string       `toml:"name"`
	APIKeyFile  string       `toml:"api_key_file"`
	Roots       []string     `toml:"roots"`
	Limits      TenantLimits `toml:"limits"`
	Credentials []Credential `toml:"credentials"`
}

// TenantLimits keep one tenant from starving the others: at most Workspaces workspaces,
// Disk of pools, templates and layers ("50GB", "500MB"; measured each maintenance round),
// and Fetches reaching the network at once. Zero is no limit, and for Fetches gitd's own.
type TenantLimits struct {
	Workspaces int    `toml:"workspaces"`
	Disk       string `toml:"disk"`
	Fetches    int    `toml:"fetches"`
	// Sandboxes is how many sandbox repositories of the endpoint it may hold at once.
	Sandboxes int `toml:"sandboxes"`
}

// DiskBytes is the disk limit in bytes, zero for none.
func (limits TenantLimits) DiskBytes() (int64, error) {
	return parseBytes(limits.Disk)
}

// HasTenants says whether gitd serves tenants; without, it is one implicit tenant, as before.
func (settings Config) HasTenants() bool {
	return len(settings.Tenants) > 0
}

// ForTenant is the configuration a tenant's part of gitd runs with: gitd's own, with the
// tenant's state directory, roots and credentials, and its fetches.
func (settings Config) ForTenant(tenant Tenant) Config {
	own := settings
	own.State = filepath.Join(settings.State, "tenants", tenant.Name)
	own.Roots = tenant.Roots
	own.Credentials = tenant.Credentials
	own.Tenants = nil
	if tenant.Limits.Fetches > 0 {
		own.Limits.Fetches = tenant.Limits.Fetches
	}
	return own
}

// normalizeTenants checks every tenant: a name, a key, roots of its own, credentials and
// limits gitd can use.
func (settings *Config) normalizeTenants() error {
	names := map[string]bool{}
	rootsOf := map[string]string{}
	for _, root := range settings.Roots {
		rootsOf[filepath.Clean(root)] = ""
	}
	for index, tenant := range settings.Tenants {
		if !tenantName.MatchString(tenant.Name) {
			return fmt.Errorf("tenants[%d]: %q is not a tenant name: lowercase letters, digits and dashes", index, tenant.Name)
		}
		if names[tenant.Name] {
			return fmt.Errorf("tenants: %q is written twice", tenant.Name)
		}
		names[tenant.Name] = true
		if tenant.APIKeyFile == "" {
			return fmt.Errorf("tenants.%s: api_key_file is needed: it is how a call names the tenant", tenant.Name)
		}
		if len(tenant.Roots) == 0 {
			return fmt.Errorf("tenants.%s: roots are needed: where its workspaces may be made", tenant.Name)
		}
		for _, root := range tenant.Roots {
			if !filepath.IsAbs(root) {
				return fmt.Errorf("tenants.%s: root %q is not absolute", tenant.Name, root)
			}
			for other, owner := range rootsOf {
				if within(root, other) || within(other, root) {
					return fmt.Errorf("tenants.%s: root %q overlaps %q of %s", tenant.Name, root, other, ownerName(owner))
				}
			}
			rootsOf[filepath.Clean(root)] = tenant.Name
		}
		if _, err := tenant.Limits.DiskBytes(); err != nil {
			return fmt.Errorf("tenants.%s: %w", tenant.Name, err)
		}
		if tenant.Limits.Workspaces < 0 || tenant.Limits.Fetches < 0 || tenant.Limits.Sandboxes < 0 {
			return fmt.Errorf("tenants.%s: limits cannot be negative", tenant.Name)
		}
		own := Config{Credentials: tenant.Credentials}
		if err := own.normalizeCredentials(); err != nil {
			return fmt.Errorf("tenants.%s: %w", tenant.Name, err)
		}
	}
	return nil
}

func ownerName(owner string) string {
	if owner == "" {
		return "gitd's roots"
	}
	return "tenant " + owner
}

// within says whether path is dir or under it.
func within(path, dir string) bool {
	relative, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, "../")
}

// parseBytes reads a size such as "50GB", "500MB" or "1048576"; empty is zero.
func parseBytes(written string) (int64, error) {
	written = strings.TrimSpace(strings.ToUpper(written))
	if written == "" {
		return 0, nil
	}
	units := []struct {
		suffix string
		size   int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}}
	for _, unit := range units {
		if number, ok := strings.CutSuffix(written, unit.suffix); ok {
			value, err := strconv.ParseInt(strings.TrimSpace(number), 10, 64)
			if err != nil || value < 0 {
				return 0, fmt.Errorf("disk %q is not a size such as 50GB", written)
			}
			return value * unit.size, nil
		}
	}
	value, err := strconv.ParseInt(written, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("disk %q is not a size such as 50GB", written)
	}
	return value, nil
}
