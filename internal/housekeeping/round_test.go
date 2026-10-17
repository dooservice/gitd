package housekeeping

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

func TestAPoolInNestedGroupsIsFoundAndReadBack(t *testing.T) {
	pools := filepath.Join(t.TempDir(), "pools")
	flat := aPool(t, pools, "web", time.Now(), "")
	nested := filepath.Join(pools, "gitlab.com", "acme", "tools", "deep", "web.git")
	halfBuilt := filepath.Join(pools, "gitlab.com", "acme", "tools", "api.git.gitd-tmp-abc")
	for _, dir := range []string{filepath.Join(nested, "objects"), halfBuilt} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	keeper := &Keeper{pools: pool.New(pools, nil, nil, nil, nil, time.Minute)}

	found := keeper.pools.All()
	slices.Sort(found)
	if want := []string{flat, nested}; !slices.Equal(found, want) {
		t.Fatalf("All = %v; want %v", found, want)
	}
	for dir, want := range map[string]repo.Repo{
		flat:   {Host: "github.com", Path: "acme/web"},
		nested: {Host: "gitlab.com", Path: "acme/tools/deep/web"},
	} {
		if got, ok := keeper.repositoryOf(dir); !ok || got != want {
			t.Errorf("repositoryOf(%s) = %v, %v; want %v", dir, got, ok, want)
		}
	}
}
