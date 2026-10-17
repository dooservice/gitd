package cli

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func TestSettingsReadAsTheirRules(t *testing.T) {
	var out bytes.Buffer
	fifty := int32(50)
	settingsLines(&out, &gitdv1.PublishSettings{
		UrlPrefix: "https://github.com/acme", Preset: "recommended", ProtectedPaths: []string{"deploy/**"},
		MaxChangedFiles: &fifty, BranchPrefix: "agents/",
		PullRequests: &gitdv1.PullRequestSettings{Open: gitdv1.PullRequestSettings_OPEN_WHEN_ASKED, Draft: true,
			Labels: []string{"agent"}, RequestReviewFromRequester: true},
	})
	settingsLines(&out, &gitdv1.PublishSettings{BranchPrefix: "agents/", FromFile: true})
	want := "https://github.com/acme\n  preset: recommended\n  protected: deploy/**\n  at most 50 files\n  branches: agents/…\n" +
		"  pull requests: when asked, drafts, labelled agent, the requester asked to review\n" +
		"every repository  (gitd.toml)\n  branches: agents/…\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestAPresetReadsAsItsRulesAndWhereItComesFrom(t *testing.T) {
	var out bytes.Buffer
	fifty := int32(50)
	presetLines(&out, &gitdv1.Preset{Name: "strict", ProtectedPaths: []string{"deploy/**"}, KeptPaths: []string{"migrations/**"},
		MaxChangedFiles: &fifty, Origin: gitdv1.Preset_ORIGIN_BUILT_IN})
	want := "strict  (built-in)\n  protected: deploy/**\n  kept: migrations/**\n  at most 50 files\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestChangesReadAsGitStatusDoesWithTheirLines(t *testing.T) {
	var out bytes.Buffer
	changesLines(&out, &gitdv1.ChangesResponse{Base: "0123456789abcdef", FromLayer: true, Files: []*gitdv1.ChangedFile{
		{Path: "src/app.go", Kind: gitdv1.ChangedFile_KIND_MODIFIED, AddedLines: 3, RemovedLines: 1},
		{Path: "logo.png", Kind: gitdv1.ChangedFile_KIND_ADDED, Binary: true},
	}})
	want := "since " + short("0123456789abcdef") + ", read from the agent's layer\n" +
		"  M  +3 -1        src/app.go\n  A  binary       logo.png\n2 files, +3 -1\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestConflictsAndEventsReadAsTheyHappened(t *testing.T) {
	var out bytes.Buffer
	conflictLines(&out, &gitdv1.ConflictsResponse{Conflicts: []*gitdv1.Conflict{{
		First: sideOf("/w/agent-2"), Second: sideOf("agents/fix"), Paths: []string{"auth.go", "go.mod"},
	}}})
	conflictLines(&out, &gitdv1.ConflictsResponse{})
	eventLine(&out, &gitdv1.WatchResponse{Kind: gitdv1.WatchResponse_KIND_PUBLISHED, Path: "/w/agent-2", Branch: "agents/fix",
		Detail: "https://github.com/acme/web/pull/7", Time: timestamppb.New(time.Date(2026, 10, 17, 12, 30, 5, 0, time.UTC))})
	want := "/w/agent-2 and agents/fix conflict on auth.go, go.mod\nevery pair merges cleanly\n" +
		"12:30:05  published /w/agent-2  agents/fix  https://github.com/acme/web/pull/7\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}
