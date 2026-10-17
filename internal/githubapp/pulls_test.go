package githubapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPullRequestIsOpenedReviewedAndLabelled(t *testing.T) {
	asked := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer ghs_x" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		asked[request.Method+" "+request.URL.Path] = body
		switch request.URL.Path {
		case "/repos/acme/web":
			_, _ = writer.Write([]byte(`{"default_branch": "main"}`))
		case "/repos/acme/web/pulls":
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"html_url": "https://github.com/acme/web/pull/128", "number": 128}`))
		default:
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	api := NewAPI(server.URL, "gitd/test")
	web := Repository{Owner: "acme", Name: "web", Token: "ghs_x"}
	ctx := context.Background()
	if base, err := api.DefaultBranch(ctx, web); err != nil || base != "main" {
		t.Fatalf("DefaultBranch = %q, %v", base, err)
	}
	opened, err := api.Open(ctx, web, PullRequest{Title: "Fix login", Body: "b", Head: "agents/fix", Base: "main", Draft: true})
	if err != nil || opened.Number != 128 || opened.URL != "https://github.com/acme/web/pull/128" {
		t.Fatalf("Open = %+v, %v", opened, err)
	}
	if pull := asked["POST /repos/acme/web/pulls"]; pull["head"] != "agents/fix" || pull["draft"] != true {
		t.Errorf("pull asked = %v", pull)
	}
	if err := api.AskToReview(ctx, web, 128, []string{"josue"}); err != nil {
		t.Fatal(err)
	}
	if err := api.Label(ctx, web, 128, []string{"agent"}); err != nil {
		t.Fatal(err)
	}
	if reviewers := asked["POST /repos/acme/web/pulls/128/requested_reviewers"]["reviewers"].([]any); reviewers[0] != "josue" {
		t.Errorf("reviewers = %v", reviewers)
	}
	if labels := asked["POST /repos/acme/web/issues/128/labels"]["labels"].([]any); labels[0] != "agent" {
		t.Errorf("labels = %v", labels)
	}
}
