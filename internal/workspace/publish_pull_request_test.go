package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/publishing"
	"github.com/dooservice/gitd/internal/pullrequests"
	"github.com/dooservice/gitd/internal/repo"
)

// openerKeeping keeps the pull request it is asked to open, and answers err when set.
type openerKeeping struct {
	asked *pullrequests.Request
	err   error
}

func (opener *openerKeeping) Open(_ context.Context, request pullrequests.Request) (*gitdv1.OpenedPullRequest, error) {
	opener.asked = &request
	if opener.err != nil {
		return nil, opener.err
	}
	return &gitdv1.OpenedPullRequest{Url: "https://github.com/acme/web/pull/7", Number: 7}, nil
}

// staged is a workspace with a commit staged for agents/fix in its pool, and the answer a
// push of it would give.
func staged(t *testing.T) (*Workspaces, repo.Repo, *gitdv1.PublishResponse) {
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
	}
}

func TestAPullRequestIsOpenedAsTheSettingsSayWithWhatGitdChecked(t *testing.T) {
	workspaces, repository, answer := staged(t)
	opener := &openerKeeping{}
	workspaces.UsePublishing(Publishing{PullRequests: opener})
	rules := publishing.Effective{Applied: []string{"https://github.com/acme"}, PullRequests: &gitdv1.PullRequestSettings{
		Open: gitdv1.PullRequestSettings_OPEN_ALWAYS, Draft: true, Labels: []string{"agent"}, RequestReviewFromRequester: true,
	}}
	request := &gitdv1.PublishRequest{Branch: "agents/fix", RequestedBy: "josue",
		PullRequest: &gitdv1.PullRequestOptions{Body: "Fixes the login."}}
	account, err := workspaces.accountOf(context.Background(), repository, request, rules)
	if err != nil {
		t.Fatal(err)
	}
	// The push marks the commits published, after which none is left to count.
	markPublished(t, workspaces, repository, "agents/fix")
	workspaces.openPullRequest(context.Background(), request, repository, rules, account, answer)

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
	workspaces, repository, answer := staged(t)
	opener := &openerKeeping{}
	workspaces.UsePublishing(Publishing{PullRequests: opener})
	asked := publishing.Effective{PullRequests: &gitdv1.PullRequestSettings{Open: gitdv1.PullRequestSettings_OPEN_WHEN_ASKED}}
	workspaces.openPullRequest(context.Background(), &gitdv1.PublishRequest{Branch: "agents/fix"}, repository, asked, pullrequests.Account{}, answer)
	if opener.asked != nil || answer.GetPullRequest() != nil {
		t.Fatal("a pull request was opened that nobody asked for")
	}
	request := &gitdv1.PublishRequest{Branch: "agents/fix", PullRequest: &gitdv1.PullRequestOptions{Open: true, Title: "Mine"}}
	workspaces.openPullRequest(context.Background(), request, repository, asked, pullrequests.Account{}, answer)
	if opener.asked == nil || opener.asked.Title != "Mine" {
		t.Errorf("asked = %+v", opener.asked)
	}
}

func TestAPullRequestThatFailsLeavesTheBranchPublishedAndSaysWhy(t *testing.T) {
	workspaces, repository, answer := staged(t)
	workspaces.UsePublishing(Publishing{PullRequests: &openerKeeping{err: git.Fail(git.AuthRefused, "no permission")}})
	request := &gitdv1.PublishRequest{Branch: "agents/fix", PullRequest: &gitdv1.PullRequestOptions{Open: true}}
	workspaces.openPullRequest(context.Background(), request, repository, publishing.Effective{}, pullrequests.Account{}, answer)
	if answer.GetPullRequestFailure().GetReason() != git.AuthRefused || answer.GetRepository() == nil {
		t.Errorf("answer = %v", answer)
	}
	if err := validRequester("not a login!"); err == nil {
		t.Error("a requester that is not an account was accepted")
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
