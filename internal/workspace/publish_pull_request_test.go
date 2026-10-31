package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/pullrequests"
	"github.com/dooservice/gitd/internal/repo"
)

// openerKeeping keeps the pull requests it is asked to open and the edits asked of them, and
// answers err when set, or refusal for a repository on its host.
type openerKeeping struct {
	asked   *pullrequests.Request
	all     []pullrequests.Request
	edited  map[int64]pullrequests.Request
	err     error
	refusal map[string]error
}

func (opener *openerKeeping) Open(_ context.Context, request pullrequests.Request) (*gitdv1.OpenedPullRequest, error) {
	opener.asked = &request
	opener.all = append(opener.all, request)
	if opener.err != nil {
		return nil, opener.err
	}
	if refused := opener.refusal[request.Repository.Host]; refused != nil {
		return nil, refused
	}
	number := int64(6 + len(opener.all))
	return &gitdv1.OpenedPullRequest{Url: fmt.Sprintf("%s/pull/%d", request.Repository.HTTPS(), number), Number: number}, nil
}

func (opener *openerKeeping) Edit(_ context.Context, request pullrequests.Request, number int64) error {
	if opener.edited == nil {
		opener.edited = map[int64]pullrequests.Request{}
	}
	opener.edited[number] = request
	return nil
}

// staged is a workspace with a commit staged for agents/fix in its pool, the answer a push
// of it would give, and the workspace's path.
func staged(t *testing.T) (*Workspaces, repo.Repo, *gitdv1.PublishResponse, string) {
	t.Helper()
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	path := made(t, workspaces, roots, "agent", repository)
	committed(t, path, map[string]string{"app.go": "one\ntwo\n"})
	commit, subjects, err := workspaces.pools.Stage(context.Background(), repository, filepath.Join(path, ".git"), "agents/fix")
	if err != nil {
		t.Fatal(err)
	}
	return workspaces, repository, &gitdv1.PublishResponse{
		Branch: "agents/fix", Repository: &gitdv1.PublishedRepository{Url: repository.HTTPS(), Commit: commit, Subjects: subjects},
	}, path
}

func TestAPullRequestIsOpenedAsTheSettingsSayWithWhatGitdChecked(t *testing.T) {
	workspaces, repository, answer, _ := staged(t)
	opener := &openerKeeping{}
	workspaces.UsePublishing(Publishing{PullRequests: opener})
	rules := publishing.Effective{Applied: []string{"https://github.com/acme"}, PullRequests: &gitdv1.PullRequestSettings{
		Open: gitdv1.PullRequestSettings_OPEN_ALWAYS, Draft: true, Labels: []string{"agent"}, RequestReviewFromRequester: true,
	}}
	request := &gitdv1.PublishRequest{Branch: "agents/fix", RequestedBy: "josue",
		PullRequest: &gitdv1.PullRequestOptions{Body: "Fixes the login."}}
	// The push marks the commits published; the branch still brings them.
	markPublished(t, workspaces, repository, "agents/fix")
	workspaces.openPullRequest(context.Background(), request, repository, rules, nil, answer)

	if answer.GetPullRequest().GetNumber() != 7 || answer.GetPullRequestFailure() != nil {
		t.Fatalf("answer = %v", answer)
	}
	asked := opener.asked
	if asked.Title != "agent" || !asked.Draft || asked.Labels[0] != "agent" || asked.Reviewers[0] != "josue" {
		t.Errorf("asked = %+v", asked)
	}
	for _, want := range []string{"Fixes the login.", "Requested by @josue", "1 file, +2 −0"} {
		if !strings.Contains(asked.Body, want) {
			t.Errorf("the body lacks %q:\n%s", want, asked.Body)
		}
	}
}

func TestNoPullRequestUnlessTheSettingsOrThePublishAsk(t *testing.T) {
	workspaces, repository, answer, _ := staged(t)
	opener := &openerKeeping{}
	workspaces.UsePublishing(Publishing{PullRequests: opener})
	asked := publishing.Effective{PullRequests: &gitdv1.PullRequestSettings{Open: gitdv1.PullRequestSettings_OPEN_WHEN_ASKED}}
	workspaces.openPullRequest(context.Background(), &gitdv1.PublishRequest{Branch: "agents/fix"}, repository, asked, nil, answer)
	if opener.asked != nil || answer.GetPullRequest() != nil {
		t.Fatal("a pull request was opened that nobody asked for")
	}
	request := &gitdv1.PublishRequest{Branch: "agents/fix", PullRequest: &gitdv1.PullRequestOptions{Open: true, Title: "Mine"}}
	workspaces.openPullRequest(context.Background(), request, repository, asked, nil, answer)
	if opener.asked == nil || opener.asked.Title != "Mine" {
		t.Errorf("asked = %+v", opener.asked)
	}
}

func TestAPullRequestThatFailsLeavesTheBranchPublishedAndSaysWhy(t *testing.T) {
	workspaces, repository, answer, _ := staged(t)
	workspaces.UsePublishing(Publishing{PullRequests: &openerKeeping{err: git.Fail(git.AuthRefused, "no permission")}})
	request := &gitdv1.PublishRequest{Branch: "agents/fix", PullRequest: &gitdv1.PullRequestOptions{Open: true}}
	workspaces.openPullRequest(context.Background(), request, repository, publishing.Effective{}, nil, answer)
	if answer.GetPullRequestFailure().GetReason() != git.AuthRefused || answer.GetRepository() == nil {
		t.Errorf("answer = %v", answer)
	}
	if err := validRequester("not a login!"); err == nil {
		t.Error("a requester that is not an account was accepted")
	}
}

func TestAnUpdateWithNothingNewGoesOnToThePullRequestOfTheBranch(t *testing.T) {
	workspaces, repository, _, path := staged(t)
	opener := &openerKeeping{}
	recorded := &entries{}
	workspaces.UsePublishing(Publishing{PullRequests: opener, Recorder: recorded})
	// An earlier publish pushed the branch, and its pull request failed.
	markPublished(t, workspaces, repository, "agents/fix")
	ctx := context.Background()
	request := &gitdv1.PublishRequest{Path: path, Branch: "agents/fix", PullRequest: &gitdv1.PullRequestOptions{Open: true}}

	_, err := workspaces.Publish(ctx, request)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.NothingNew || opener.asked != nil {
		t.Fatalf("publishing again without update = %v; want nothing new, and no pull request", err)
	}
	request.Update = true
	answer, err := workspaces.Publish(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if answer.GetPullRequest().GetNumber() != 7 || len(answer.GetRepository().GetSubjects()) != 0 {
		t.Errorf("answer = %v; want the pull request, and nothing pushed", answer)
	}
	if asked := opener.asked; !asked.Update || asked.Title != "agent" || !strings.Contains(asked.Body, "1 file, +2 −0") {
		t.Errorf("asked = %+v; want an update telling the whole branch", asked)
	}
	if len(recorded.kept) != 2 || recorded.kept[1].GetRefusal() != nil || recorded.kept[1].GetPullRequest() == nil {
		t.Errorf("recorded = %v", recorded.kept)
	}

	// Back on main, the workspace brings nothing the branch holds.
	run(t, path, "reset", "-q", "--hard", "HEAD~1")
	if _, err := workspaces.Publish(ctx, request); err == nil {
		t.Error("an update bringing nothing at all was published")
	}
}

func TestEachSubmoduleHasItsPullRequestMergedFirstAndLinkedBack(t *testing.T) {
	web := repo.Repo{Host: "github.com", Path: "acme/web"}
	auth := repo.Repo{Host: "github.com", Path: "acme/auth"}
	tools := repo.Repo{Host: "gitlab.com", Path: "acme/tools"}
	workspaces, roots := newTestWorkspaces(t, web, auth, tools)
	opener := &openerKeeping{refusal: map[string]error{"gitlab.com": git.Fail(git.Unsupported, "not GitHub")}}
	recorded := &entries{}
	workspaces.UsePublishing(Publishing{PullRequests: opener, Recorder: recorded})
	ctx := context.Background()
	var submodules []stagedSubmodule
	for _, repository := range []repo.Repo{web, auth, tools} {
		path := made(t, workspaces, roots, filepath.Base(repository.Path), repository)
		committed(t, path, map[string]string{repository.Path + ".go": "fix\n"})
		if _, _, err := workspaces.pools.Stage(ctx, repository, filepath.Join(path, ".git"), "agents/fix"); err != nil {
			t.Fatal(err)
		}
		if repository != web {
			submodules = append(submodules, stagedSubmodule{repository: repository, fresh: true, base: map[string]string{
				"acme/auth": "16.0", "acme/tools": ".",
			}[repository.Path], published: &gitdv1.PublishedRepository{Path: "libs/" + filepath.Base(repository.Path)}})
		}
	}
	request := &gitdv1.PublishRequest{Branch: "agents/fix", PullRequest: &gitdv1.PullRequestOptions{
		Open: true, Submodules: true, Title: "Fix login", Body: "Fixes the login.", Base: "main",
	}}
	answer := &gitdv1.PublishResponse{Branch: "agents/fix"}
	workspaces.openPullRequest(ctx, request, web, publishing.Effective{}, submodules, answer)
	workspaces.record(&gitdv1.AuditEntry{Branch: "agents/fix"}, answer, nil)

	if len(opener.all) != 3 || opener.all[0].Repository != auth || opener.all[2].Repository != web {
		t.Fatalf("opened %v; want the submodules' first", opener.all)
	}
	if first := opener.all[0]; first.Base != "16.0" || first.Title != "Fix login" || !strings.HasPrefix(first.Body, "Fixes the login.") {
		t.Errorf("auth's pull request = %+v", first)
	}
	if opener.all[1].Base != "main" {
		t.Errorf("tools' pull request is into %q; want the superproject's base, as `.` says", opener.all[1].Base)
	}
	said := answer.GetSubmodulePullRequests()
	if len(said) != 2 || said[0].GetPath() != "libs/auth" || said[0].GetUrl() != auth.HTTPS() || said[0].GetPullRequest().GetNumber() != 7 ||
		said[1].GetFailure().GetReason() != git.Unsupported || said[1].GetPullRequest() != nil {
		t.Errorf("submodule pull requests = %v", said)
	}
	body := opener.all[2].Body
	if !strings.Contains(body, "Merge these first, each with a merge commit, not squash or rebase") ||
		!strings.Contains(body, "- https://github.com/acme/auth/pull/7\n") || strings.Contains(body, "tools/pull") {
		t.Errorf("the superproject's body:\n%s", body)
	}
	linked := opener.edited[7]
	if len(opener.edited) != 1 || !strings.Contains(linked.Body, "Part of https://github.com/acme/web/pull/9: merge this one first, with a merge commit.") ||
		linked.Title != "Fix login" || linked.Repository != auth {
		t.Errorf("edited = %v", opener.edited)
	}
	if len(recorded.kept[0].GetSubmodulePullRequests()) != 2 {
		t.Errorf("recorded = %v", recorded.kept[0])
	}
}

func TestASubmoduleNamesTheBranchItFollows(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	path := made(t, workspaces, roots, "agent", repository)
	writeFile(t, filepath.Join(path, ".gitmodules"),
		"[submodule \"auth\"]\n\tpath = libs/auth\n\turl = ../auth\n\tbranch = 16.0\n[submodule \"ui\"]\n\tpath = ui\n\turl = ../ui\n")
	declared, _, err := workspaces.declaredIn(context.Background(), path)
	if err != nil || len(declared) != 2 || declared[0].branch != "16.0" || declared[0].url != "../auth" || declared[1].branch != "" {
		t.Errorf("declared = %+v, %v", declared, err)
	}
}

// markPublished does what a push does to the pool: the staged commits become published.
func markPublished(t *testing.T, workspaces *Workspaces, repository repo.Repo, branch string) {
	t.Helper()
	run(t, workspaces.pools.Path(repository), "update-ref", "refs/gitd/published/"+branch, "refs/gitd/staging/"+branch)
}

func TestTheCommitAWorkspaceIsPutOnIsNotedAndForgottenOnRemove(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	path := made(t, workspaces, roots, "agent", repository)
	head := run(t, path, "rev-parse", "HEAD")
	if base, found := workspaces.pools.Base(repository, path); !found || base != head {
		t.Fatalf("base = %q, %v; want %s", base, found, head)
	}
	if err := workspaces.Remove(context.Background(), &gitdv1.RemoveRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	if _, found := workspaces.pools.Base(repository, path); found {
		t.Error("a removed workspace's base is still noted")
	}
}

func TestAProvenanceIsKeptAsToldAndCheckedFirst(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	recorded := &entries{}
	workspaces.UsePublishing(Publishing{Recorder: recorded})
	path := made(t, workspaces, roots, "agent", repository)
	origin := &gitdv1.Provenance{Agent: "openhands", Model: "claude-opus-5-5", TaskUrl: "https://github.com/acme/web/issues/7"}
	_, _ = workspaces.Publish(context.Background(), &gitdv1.PublishRequest{Path: path, Branch: "agents/x", Provenance: origin})
	if got := recorded.kept[0].GetProvenance(); got.GetAgent() != "openhands" || got.GetTaskUrl() != origin.GetTaskUrl() {
		t.Errorf("recorded provenance = %v", got)
	}
	for _, bad := range []*gitdv1.Provenance{{TaskUrl: "http://example.com/1"}, {Agent: "two\nlines"}, {Model: strings.Repeat("m", 201)}} {
		_, err := workspaces.Publish(context.Background(), &gitdv1.PublishRequest{Path: path, Branch: "agents/x", Provenance: bad})
		if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.BadAddress {
			t.Errorf("%v: %v", bad, err)
		}
	}
}
