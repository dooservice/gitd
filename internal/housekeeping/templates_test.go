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
