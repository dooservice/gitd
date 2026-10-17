package pullrequests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dooservice/gitd/internal/connections"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

func TestAPullRequestIsOpenedWithTheCredentialThatReachesTheRepository(t *testing.T) {
	asked := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer ghp_kept" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		asked[request.URL.Path] = body
		switch request.URL.Path {
		case "/repos/acme/web":
			_, _ = writer.Write([]byte(`{"default_branch": "main"}`))
		case "/repos/acme/web/pulls":
			_, _ = writer.Write([]byte(`{"html_url": "https://github.com/acme/web/pull/7", "number": 7}`))
		case "/repos/acme/web/pulls/7/requested_reviewers":
			writer.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = writer.Write([]byte(`{"message": "Reviews may only be requested from collaborators."}`))
		default:
			_, _ = writer.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	keys, err := credentials.NewKeys(t.TempDir(), nil, nil, []credentials.Credential{credentials.KeptToken("https://github.com/acme", "ghp_kept", "")})
	if err != nil {
		t.Fatal(err)
	}
	opener := New(keys, connections.GitHub{UserAgent: "gitd/test", APIBase: server.URL})
	web, _ := repo.Of("https://github.com/acme/web")
	opened, err := opener.Open(context.Background(), Request{
		Repository: web, Branch: "agents/fix", Title: "Fix login", Body: "body", Draft: true,
		Labels: []string{"agent"}, Reviewers: []string{"josue"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.GetNumber() != 7 || asked["/repos/acme/web/pulls"]["base"] != "main" || asked["/repos/acme/web/pulls"]["head"] != "agents/fix" {
		t.Errorf("opened = %v, asked %v", opened, asked)
	}
	if asked["/repos/acme/web/issues/7/labels"] == nil {
		t.Error("the label was not asked for after a review was refused")
	}
}

func TestAPullRequestElsewhereThanGitHubIsUnsupported(t *testing.T) {
	keys, err := credentials.NewKeys(t.TempDir(), nil, nil, []credentials.Credential{credentials.KeptToken("https://gitlab.com/acme", "glpat", "")})
	if err != nil {
		t.Fatal(err)
	}
	tools, _ := repo.Of("https://gitlab.com/acme/tools")
	_, err = New(keys, connections.GitHub{}).Open(context.Background(), Request{Repository: tools, Branch: "agents/x"})
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.Unsupported {
		t.Errorf("err = %v", err)
	}
}
