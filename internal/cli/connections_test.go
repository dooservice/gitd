package cli

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func TestAConnectionReadsAsItsKindPrefixAndWhomItActsAs(t *testing.T) {
	var out bytes.Buffer
	connectionLine(&out, &gitdv1.Connection{Id: "login-1", Kind: gitdv1.Connection_KIND_GITHUB_LOGIN,
		UrlPrefix: "https://github.com", ActsAs: "josue", Expires: timestamppb.New(time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC))})
	connectionLine(&out, &gitdv1.Connection{Id: "file-1", Kind: gitdv1.Connection_KIND_TOKEN, UrlPrefix: "https://github.com/acme", FromFile: true})
	want := "login-1  github_login  https://github.com  as josue  until 2027-04-01\nfile-1  token  https://github.com/acme  (gitd.toml)\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestTheAuditReadsChangesPullRequestsAndWhoAsked(t *testing.T) {
	var out bytes.Buffer
	when := timestamppb.New(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	auditLine(&out, &gitdv1.AuditEntry{Time: when, Change: &gitdv1.ConfigurationChange{
		Action: gitdv1.ConfigurationChange_ACTION_CONNECTION_ADDED, Subject: "app-1", UrlPrefix: "https://github.com/acme",
	}})
	auditLine(&out, &gitdv1.AuditEntry{Time: when, Branch: "agents/fix", Url: "https://github.com/acme/web", Workspace: "/w",
		RequestedBy: "josue", Repository: &gitdv1.PublishedRepository{Url: "https://github.com/acme/web", Commit: "abcdef1234", Subjects: []string{"fix"}},
		PullRequest: &gitdv1.OpenedPullRequest{Url: "https://github.com/acme/web/pull/7", Number: 7}})
	auditLine(&out, &gitdv1.AuditEntry{Time: when, Change: &gitdv1.ConfigurationChange{
		Action: gitdv1.ConfigurationChange_ACTION_PRESET_SET, Subject: "strict",
	}})
	want := "2026-10-17 12:00:00  connection added  app-1  https://github.com/acme\n" +
		"2026-10-17 12:00:00  agents/fix  https://github.com/acme/web  /w\n  requested by josue\n" +
		"  published https://github.com/acme/web " + short("abcdef1234") + ": fix\n  pull request #7: https://github.com/acme/web/pull/7\n" +
		"2026-10-17 12:00:00  preset set  strict\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}
