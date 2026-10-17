package connectpage

import (
	"bytes"
	"strings"
	"testing"
)

func TestTheCreatePageShowsTheAppAndPostsItsManifestEscaped(t *testing.T) {
	var out bytes.Buffer
	if err := Create(&out, "127.0.0.1:1", App{Name: "acme-agents", Organization: "acme"},
		"https://github.com/organizations/acme/settings/apps/new?state=s", `{"name":"<script>"}`); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	for _, want := range []string{
		"acme-agents[bot]", "Private to acme", `action="https://github.com/organizations/acme/settings/apps/new?state=s"`,
		`name="manifest"`, "--accent: #2B7F8A",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(page, `"<script>"`) {
		t.Error("the manifest is not escaped")
	}
}

func TestAPersonalAppIsOwnedByTheirAccount(t *testing.T) {
	var out bytes.Buffer
	if err := Create(&out, "127.0.0.1:1", App{Name: "mine"}, "https://github.com/settings/apps/new?state=s", "{}"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "owned by <strong>your account</strong>") {
		t.Error("a personal App does not say it is the person's")
	}
}

func TestTheConnectedAndRefusedPagesSayWhatHappened(t *testing.T) {
	var connected, refused bytes.Buffer
	if err := WriteConnected(&connected, "127.0.0.1:1", Connected{ID: "app-1", ActsAs: "acme-agents[bot]",
		Prefix: "https://github.com/acme", InstallURL: "https://github.com/apps/acme-agents/installations/new"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(connected.String(), `href="https://github.com/apps/acme-agents/installations/new"`) ||
		strings.Contains(connected.String(), "Manage the App") {
		t.Error("the connected page lacks the install link, or shows a manage link it was not given")
	}
	if err := Refused(&refused, "127.0.0.1:1", App{Name: "acme-agents", Organization: "acme"}, "Not kept.", "GitHub answered 404"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(refused.String(), "GitHub answered 404") || !strings.Contains(refused.String(), "--org acme") {
		t.Error("the refused page lacks why, or how to try again")
	}
}
