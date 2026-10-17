package workspace

import (
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

func TestOnlyANewWorkspaceMadeByOverlayStandsOnAWarmLayer(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	work := job{path: filepath.Join(roots, "agent-1"), repository: repository, revision: onBranch("main"), warm: "deps"}
	workspaces.UseTemplates(nil, nil)
	if err := workspaces.canStandOnWarm(work, false); !hasReason(err, git.Unsupported) {
		t.Errorf("a gitd without overlay would stand a workspace on a warm layer: %v", err)
	}
	state := t.TempDir()
	workspaces.UseTemplates(template.New(filepath.Join(state, "templates"), workspaces.locks, workspaces),
		&strategy.OverlayMaker{Layers: filepath.Join(state, "layers"), Mounter: treeMounter{}})
	if err := workspaces.canStandOnWarm(work, false); err != nil {
		t.Errorf("a new workspace by overlay cannot stand on a warm layer: %v", err)
	}
	if err := workspaces.canStandOnWarm(work, true); !hasReason(err, git.Unsupported) {
		t.Errorf("an existing workspace would move onto a warm layer: %v", err)
	}
	full := work
	full.full = true
	if err := workspaces.canStandOnWarm(full, false); !hasReason(err, git.Unsupported) {
		t.Errorf("a whole-history workspace would stand on a warm layer: %v", err)
	}
	named := work
	named.warm = "../deps"
	if err := workspaces.canStandOnWarm(named, false); !hasReason(err, git.BadAddress) {
		t.Errorf("a warm layer named %q was looked for: %v", named.warm, err)
	}
	without := work
	without.warm = ""
	without.full = true
	if err := workspaces.canStandOnWarm(without, true); err != nil {
		t.Errorf("a workspace asking for no warm layer was refused: %v", err)
	}
}
