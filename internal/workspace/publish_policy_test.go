package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/repo"
)

// committed writes files in a workspace and commits them.
func committed(t *testing.T, path string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		file := filepath.Join(path, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(t, path, "add", "-A")
	run(t, path, "commit", "-q", "-m", "agent")
}

func TestAPublishThatBreaksARuleIsRefusedBeforeAnythingIsPushed(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	settings, err := publishing.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	two := int32(2)
	if _, err := settings.Set(&gitdv1.PublishSettings{
		UrlPrefix: "https://github.com/acme", Preset: "recommended", MaxChangedFiles: &two,
	}); err != nil {
		t.Fatal(err)
	}
	recorded := &entries{}
	workspaces.UsePublishing(Publishing{Settings: settings, Recorder: recorded})
	ctx := context.Background()

	cases := []struct {
		name, refusal string
		files         map[string]string
		reason        git.Reason
	}{
		{"ci", ".github/workflows/ci.yml", map[string]string{".github/workflows/ci.yml": "evil\n"}, git.PolicyRefused},
		{"too-many", "changes 3 files", map[string]string{"a.go": "a", "b.go": "b", "c.go": "c"}, git.PolicyRefused},
		{"fine", "", map[string]string{"app.go": "fix\n"}, git.AuthRequired},
	}
	for _, attempt := range cases {
		path := made(t, workspaces, roots, attempt.name, repository)
		committed(t, path, attempt.files)
		_, err := workspaces.Publish(ctx, &gitdv1.PublishRequest{Path: path, Branch: "agents/" + attempt.name})
		failure, ok := git.AsFailure(err)
		if !ok || failure.Reason != attempt.reason || !strings.Contains(failure.GitSaid, attempt.refusal) {
			t.Errorf("%s: %v; want %v saying %q", attempt.name, err, attempt.reason, attempt.refusal)
		}
	}
	if len(recorded.kept) != len(cases) || recorded.kept[0].GetRefusal().GetReason() != git.PolicyRefused {
		t.Errorf("recorded = %v", recorded.kept)
	}
}
