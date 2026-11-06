package userservice

import (
	"encoding/xml"
	"slices"
	"strings"
	"testing"
)

func TestGitdStartsItselfOrThroughShReadingTheGitHubCLIsToken(t *testing.T) {
	plain := Service{Executable: "/usr/local/bin/gitd", Config: "/home/ana/.config/gitd/gitd.toml"}
	if got, want := plain.command(), []string{"/usr/local/bin/gitd", "serve", "--config", "/home/ana/.config/gitd/gitd.toml"}; !slices.Equal(got, want) {
		t.Errorf("without gh, %q; want %q", got, want)
	}
	withGitHub := plain
	withGitHub.GitHubCLI = "/usr/bin/gh"
	got := withGitHub.command()
	if want := []string{"/bin/sh", "-c", tokenScript, "/usr/bin/gh", "/usr/local/bin/gitd", "/home/ana/.config/gitd/gitd.toml"}; !slices.Equal(got, want) {
		t.Errorf("with gh, %q; want %q", got, want)
	}
}

func TestTheUnitDoublesSpecifiersAndVariablesAndQuotesWhatHasSpaces(t *testing.T) {
	unit := systemdUnit{service: Service{
		Executable: "/opt/my tools/gitd",
		Config:     "/home/ana/100%/gitd.toml",
		GitHubCLI:  "/usr/bin/gh",
	}}.String()
	want := `ExecStart=/bin/sh -c "GH_TOKEN=\"$$(\"$$0\" auth token)\"; export GH_TOKEN; exec \"$$1\" serve --config \"$$2\"" ` +
		`/usr/bin/gh "/opt/my tools/gitd" /home/ana/100%%/gitd.toml` + "\n"
	if !strings.Contains(unit, want) {
		t.Errorf("the unit\n%s\nhas no line\n%s", unit, want)
	}
	for _, line := range []string{"Restart=on-failure\n", "WantedBy=default.target\n"} {
		if !strings.Contains(unit, line) {
			t.Errorf("the unit has no %q", line)
		}
	}
}

func TestTheAgentIsAPropertyListWithItsArgumentsEscaped(t *testing.T) {
	agent := launchAgent{
		service: Service{Executable: "/usr/local/bin/gitd", Config: "/Users/ana/Library/Application Support/gitd/a&b.toml"},
		log:     "/Users/ana/Library/Logs/gitd.log",
	}.String()
	var plist struct {
		Dict struct {
			Keys    []string `xml:"key"`
			Strings []string `xml:"string"`
			Array   struct {
				Strings []string `xml:"string"`
			} `xml:"array"`
		} `xml:"dict"`
	}
	if err := xml.Unmarshal([]byte(agent), &plist); err != nil {
		t.Fatalf("not XML: %v\n%s", err, agent)
	}
	want := []string{"/usr/local/bin/gitd", "serve", "--config", "/Users/ana/Library/Application Support/gitd/a&b.toml"}
	if got := plist.Dict.Array.Strings; !slices.Equal(got, want) {
		t.Errorf("ProgramArguments %q; want %q", got, want)
	}
	if !slices.Contains(plist.Dict.Strings, Label) || !slices.Contains(plist.Dict.Keys, "RunAtLoad") {
		t.Errorf("no label or no RunAtLoad:\n%s", agent)
	}
}
