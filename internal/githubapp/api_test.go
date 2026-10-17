package githubapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTheAPIOfAHostIsWhereGitHubServesIt(t *testing.T) {
	for host, want := range map[string]string{
		"github.com":         "https://api.github.com",
		"":                   "https://api.github.com",
		"github.example.com": "https://github.example.com/api/v3",
	} {
		if got := APIBase(host, 0); got != want {
			t.Errorf("APIBase(%q) = %q; want %q", host, got, want)
		}
	}
	if got := APIBase("github.example.com", 8443); got != "https://github.example.com:8443/api/v3" {
		t.Errorf("on another port: %q", got)
	}
}

func TestACallSendsWhatGitHubAsksForAndReadsItsAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer jwt" || request.Header.Get("X-GitHub-Api-Version") != apiVersion ||
			request.Header.Get("User-Agent") != "gitd/test" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte(`{"id": 42}`))
	}))
	defer server.Close()
	var answer struct {
		ID int64 `json:"id"`
	}
	if err := NewAPI(server.URL, "gitd/test").call(context.Background(), http.MethodGet, "/x", "Bearer jwt", nil, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.ID != 42 {
		t.Errorf("answer = %+v", answer)
	}
}

func TestAFailureSaysWhatGitHubSaidAndARedirectIsNotFollowed(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the redirect was followed, with the token")
	}))
	defer elsewhere.Close()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/moved" {
			http.Redirect(writer, request, elsewhere.URL, http.StatusFound)
			return
		}
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()
	api := NewAPI(server.URL, "gitd/test")
	var failure *APIError
	if err := api.call(context.Background(), http.MethodGet, "/missing", "", nil, nil); !errors.As(err, &failure) ||
		failure.Status != http.StatusNotFound || failure.Message != "Not Found" {
		t.Errorf("err = %v", err)
	}
	if err := api.call(context.Background(), http.MethodGet, "/moved", "token x", nil, nil); !errors.As(err, &failure) ||
		failure.Status != http.StatusFound {
		t.Errorf("redirect: %v", err)
	}
}

func TestAValidationFailureSaysWhatWasInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{"message": "Validation Failed", "errors": [
			{"resource": "PullRequest", "code": "custom", "message": "Draft pull requests are not supported in this repository."},
			{"resource": "PullRequest", "field": "base", "code": "invalid"}]}`))
	}))
	defer server.Close()
	err := NewAPI(server.URL, "gitd/test").call(context.Background(), http.MethodPost, "/x", "", nil, nil)
	want := "GitHub answered 422: Validation Failed: Draft pull requests are not supported in this repository.; base invalid"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}
