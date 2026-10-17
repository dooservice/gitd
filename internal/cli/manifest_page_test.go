package cli

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func TestTheManifestPagePostsToGitHubAndTakesTheCodeOnlyWithItsState(t *testing.T) {
	page, err := newManifestPage(0)
	if err != nil {
		t.Fatal(err)
	}
	defer page.close()
	open := page.serve(&gitdv1.BeginGitHubAppResponse{
		FormAction: "https://github.com/organizations/acme/settings/apps/new?state=s1",
		Manifest:   `{"name":"acme-agents","x":"<script>"}`, State: "s1",
	})
	response, err := http.Get(open)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), `action="https://github.com/organizations/acme/settings/apps/new?state=s1"`) ||
		strings.Contains(string(body), `"<script>"`) {
		t.Errorf("page:\n%s", body)
	}
	if response, err := http.Get(page.redirectURL() + "?state=guessed&code=c"); err != nil || response.StatusCode != http.StatusBadRequest {
		t.Errorf("a guessed state: %v %v", response.StatusCode, err)
	}
	landed := make(chan string, 1)
	go func() {
		response, err := http.Get(page.redirectURL() + "?state=s1&code=code-1")
		if err != nil {
			landed <- err.Error()
			return
		}
		body, _ := io.ReadAll(response.Body)
		landed <- string(body)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, err := page.waitForCode(ctx)
	if err != nil || code != "code-1" {
		t.Fatalf("code = %q, %v", code, err)
	}
	page.finish(&gitdv1.CompleteGitHubAppResponse{
		Connection: &gitdv1.Connection{ActsAs: "acme-agents[bot]"}, InstallUrl: "https://github.com/apps/acme-agents/installations/new",
	}, nil)
	if text := <-landed; !strings.Contains(text, "gitd keeps acme-agents[bot]") {
		t.Errorf("landed on:\n%s", text)
	}
}
