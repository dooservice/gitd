package housekeeping

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// stubBuilder checks a template out as one empty directory.
type stubBuilder struct{}

func (stubBuilder) BuildTemplate(_ context.Context, _ template.Key, building, _ string) error {
	return os.MkdirAll(building, 0o755)
}

func (stubBuilder) ForgetTemplate(context.Context, template.Key, string) error { return nil }

func TestARoundRetiresTheTemplatesNobodyUses(t *testing.T) {
	root := t.TempDir()
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	templates := template.New(filepath.Join(root, "templates"), locks, stubBuilder{})
	key := template.Key{Repository: repo.Repo{Host: "github.com", Path: "acme/web"}, Commit: "4efeed310944064485f024fefa3ab3c8021be23d"}
	dir, err := templates.Take(context.Background(), key, filepath.Join(root, "gone"))
	if err != nil {
		t.Fatal(err)
	}
	keeper := &Keeper{
		pools:     pool.New(filepath.Join(root, "pools"), nil, nil, locks, nil, time.Minute),
		templates: templates,
		settings:  config.Housekeeping{TemplateGrace: 0},
		metrics:   metrics.New(func() float64 { return 0 }),
	}
	keeper.round(context.Background())
	if _, err := os.Stat(dir); err == nil {
		t.Error("a template nobody uses outlived the round")
	}
}

func TestTheSweepFreesTheTemplateOfAWorkspaceThatIsGoneAndProbesLeftBehind(t *testing.T) {
	root := t.TempDir()
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	templates := template.New(filepath.Join(root, "state", "templates"), locks, stubBuilder{})
	key := template.Key{Repository: repo.Repo{Host: "github.com", Path: "acme/web"}, Commit: "4efeed310944064485f024fefa3ab3c8021be23d"}
	workspace := filepath.Join(root, "workspaces", "agent-1")
	dir, err := templates.Take(context.Background(), key, workspace)
	if err != nil {
		t.Fatal(err)
	}
	overlay := &strategy.OverlayMaker{Layers: filepath.Join(root, "state", "layers"), Mounter: noMounter{}}
	if err := overlay.Make(context.Background(), dir, workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(workspace); err != nil { // deleted by hand, unmounted
		t.Fatal(err)
	}
	probes := []string{
		filepath.Join(root, "workspaces", ".gitd-probe-123"),
		filepath.Join(root, "state", "templates", ".probe-456"),
	}
	for _, probe := range probes {
		if err := os.MkdirAll(probe, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	keeper := &Keeper{
		pools:     pool.New(filepath.Join(root, "state", "pools"), nil, nil, locks, nil, time.Minute),
		templates: templates,
		overlay:   overlay,
		roots:     []string{filepath.Join(root, "workspaces")},
		settings:  config.Housekeeping{SweepDepth: 4},
	}
	keeper.Sweep()
	if users := templates.Users(key); len(users) != 0 {
		t.Errorf("the template still counts the gone workspace: %v", users)
	}
	if len(overlay.Records()) != 0 {
		t.Error("the gone workspace's layer is still there")
	}
	for _, probe := range probes {
		if _, err := os.Stat(probe); err == nil {
			t.Errorf("%s left behind", probe)
		}
	}
}

// noMounter mounts nothing: enough for a layer to be recorded.
type noMounter struct{}

func (noMounter) MountOverlay(string, string, string, string) error { return nil }
func (noMounter) Unmount(string) error                              { return nil }
