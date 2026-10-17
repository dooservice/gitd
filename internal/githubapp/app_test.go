package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/repo"
)

// fakeGitHub answers as GitHub does for an App installed on acme/web only, counting the
// installation lookups and the tokens it makes, and keeping what each token was asked for.
type fakeGitHub struct {
	lookups atomic.Int32
	made    atomic.Int32
	guard   sync.Mutex
	asked   []map[string]any
	expires time.Time
}

func (fake *fakeGitHub) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/installation":
		fake.lookups.Add(1)
		_, _ = writer.Write([]byte(`{"id": 77}`))
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/installation"):
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"message": "Not Found"}`))
	case request.Method == http.MethodPost && request.URL.Path == "/app/installations/77/access_tokens":
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		fake.guard.Lock()
		fake.asked = append(fake.asked, body)
		fake.guard.Unlock()
		number := fake.made.Add(1)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"token": "ghs_" + string(rune('a'+number-1)), "expires_at": fake.expires.Format(time.RFC3339),
		})
	case request.Method == http.MethodGet && request.URL.Path == "/app":
		_, _ = writer.Write([]byte(`{"slug": "acme-agents", "html_url": "https://github.com/apps/acme-agents"}`))
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func testApp(t *testing.T, fake *fakeGitHub) (*App, *time.Time) {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	data, _ := testKey(t)
	signer, err := NewSigner(1, func() ([]byte, error) { return data, nil })
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	fake.expires = now.Add(time.Hour)
	app := NewApp(signer, NewAPI(server.URL, "gitd/test"))
	app.now = func() time.Time { return now }
	return app, &now
}

func TestATokenIsForOneRepositoryAndWhatTheOperationNeeds(t *testing.T) {
	fake := &fakeGitHub{}
	app, _ := testApp(t, fake)
	web, _ := repo.Of("https://github.com/acme/web")
	for _, purpose := range []credentials.Purpose{credentials.Read, credentials.Write, credentials.PullRequests} {
		if _, err := app.Token(context.Background(), web, purpose); err != nil {
			t.Fatal(err)
		}
	}
	want := []map[string]any{
		{"contents": "read", "metadata": "read"},
		{"contents": "write", "metadata": "read"},
		{"pull_requests": "write", "contents": "read", "metadata": "read"},
	}
	for index, asked := range fake.asked {
		repositories := asked["repositories"].([]any)
		if len(repositories) != 1 || repositories[0] != "web" || !maps.Equal(asked["permissions"].(map[string]any), want[index]) {
			t.Errorf("token %d asked for %v; want %v", index, asked, want[index])
		}
	}
}

func TestAHundredOperationsAtOnceAskForOneToken(t *testing.T) {
	fake := &fakeGitHub{}
	app, _ := testApp(t, fake)
	web, _ := repo.Of("https://github.com/acme/web")
	var waiting sync.WaitGroup
	for range 100 {
		waiting.Go(func() {
			if _, err := app.Token(context.Background(), web, credentials.Read); err != nil {
				t.Error(err)
			}
		})
	}
	waiting.Wait()
	if fake.lookups.Load() != 1 || fake.made.Load() != 1 {
		t.Errorf("%d lookups and %d tokens for 100 operations", fake.lookups.Load(), fake.made.Load())
	}
}

func TestATokenIsRenewedBeforeItExpires(t *testing.T) {
	fake := &fakeGitHub{}
	app, now := testApp(t, fake)
	web, _ := repo.Of("https://github.com/acme/web")
	first, _ := app.Token(context.Background(), web, credentials.Read)
	*now = now.Add(50 * time.Minute)
	if again, _ := app.Token(context.Background(), web, credentials.Read); again != first {
		t.Error("a token with ten minutes left was not used")
	}
	*now = now.Add(6 * time.Minute)
	fake.expires = now.Add(time.Hour)
	if renewed, _ := app.Token(context.Background(), web, credentials.Read); renewed == first {
		t.Error("a token with four minutes left was used")
	}
}

func TestARepositoryTheAppIsNotInstalledOnSaysSo(t *testing.T) {
	app, _ := testApp(t, &fakeGitHub{})
	other, _ := repo.Of("https://github.com/other/tools")
	if _, err := app.Token(context.Background(), other, credentials.Read); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v", err)
	}
}

func TestTheAppSaysWhichBotItActsAs(t *testing.T) {
	app, _ := testApp(t, &fakeGitHub{})
	bot, manage, err := app.Identity(context.Background())
	if err != nil || bot != "acme-agents[bot]" || manage != "https://github.com/apps/acme-agents" {
		t.Errorf("Identity = %q, %q, %v", bot, manage, err)
	}
}
