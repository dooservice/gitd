package workspace

import (
	"context"
	"path/filepath"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/repo"
)

// TestWhatASandboxPushedIsPublishedUnderTheSameRules publishes from a git directory as from
// a sandbox repository: a branch it never pushed, a branch with nothing new, one breaking a
// rule, and one the host is asked to take, each recorded as the sandbox's.
func TestWhatASandboxPushedIsPublishedUnderTheSameRules(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	settings, err := publishing.Open(t.TempDir(), publishing.File{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Set(&gitdv1.PublishSettings{
		UrlPrefix: "https://github.com/acme", Preset: "recommended", BranchPrefix: "agents/",
	}); err != nil {
		t.Fatal(err)
	}
	recorded := &entries{}
	workspaces.UsePublishing(Publishing{Settings: settings, Recorder: recorded})
	ctx := context.Background()
	pushed := made(t, workspaces, roots, "pushed", repository)
	source := filepath.Join(pushed, ".git")
	run(t, pushed, "branch", "agents/nothing")
	committed(t, pushed, map[string]string{".github/workflows/ci.yml": "evil\n"})
	run(t, pushed, "branch", "agents/ci")
	run(t, pushed, "reset", "-q", "--hard", "HEAD~1")
	committed(t, pushed, map[string]string{"fix.go": "package fix\n"})
	run(t, pushed, "branch", "agents/fix")

	for branch, reason := range map[string]git.Reason{
		"agents/never":   git.NotFound,
		"agents/nothing": git.NothingNew,
		"agents/ci":      git.PolicyRefused,
		"main-of-mine":   git.BranchRefused,
		"agents/fix":     git.AuthRequired,
	} {
		_, err := workspaces.PublishPushed(ctx, &gitdv1.PublishRequest{Sandbox: "sbx_0123456789abcdef", Branch: branch}, repository, source)
		if failure, ok := git.AsFailure(err); !ok || failure.Reason != reason {
			t.Errorf("%s: %v; want %v", branch, err, reason)
		}
	}
	if staged := run(t, workspaces.pools.Path(repository), "for-each-ref", "refs/gitd/staging/"); staged != "" {
		t.Errorf("refused publishes left their commits staged in the pool:\n%s", staged)
	}
	if len(recorded.kept) != 5 || recorded.kept[0].GetSandbox() != "sbx_0123456789abcdef" || recorded.kept[0].GetUrl() != repository.HTTPS() {
		t.Errorf("recorded = %v", recorded.kept)
	}
}
