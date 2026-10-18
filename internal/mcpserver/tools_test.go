package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dooservice/gitd/internal/client"
)

// listed is the tools an agent sees, through a real MCP session.
func listed(t *testing.T) []*mcp.Tool {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "gitd", Version: "test"}, nil)
	(&tools{gitd: client.New("/nonexistent.sock")}).register(server)
	serverSide, clientSide := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverSide, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	answer, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return answer.Tools
}

func TestNoToolEverAsksTheAgentForACredential(t *testing.T) {
	for _, tool := range listed(t) {
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range []string{"token", "credential", "password", "secret", "ssh_key", "key_path"} {
			if strings.Contains(strings.ToLower(string(schema)), word) {
				t.Errorf("%s takes %q: an agent must never handle a credential\n%s", tool.Name, word, schema)
			}
		}
	}
}

func TestEveryToolSaysWhetherItReadsOrDestroys(t *testing.T) {
	want := map[string]struct{ readOnly, destructive bool }{
		"prepare_workspace":    {false, false},
		"update_workspace":     {false, true},
		"checkout_commit":      {false, true},
		"describe_workspace":   {true, false},
		"list_changes":         {true, false},
		"show_diff":            {true, false},
		"publish_branch":       {false, false},
		"remove_workspace":     {false, true},
		"probe_repository":     {true, false},
		"open_endpoint_access": {false, false},
		"read_audit_log":       {true, false},
	}
	got := listed(t)
	if len(got) != len(want) {
		t.Fatalf("%d tools; want %d", len(got), len(want))
	}
	for _, tool := range got {
		expected, ok := want[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %s", tool.Name)
			continue
		}
		destructive := tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
		if tool.Annotations.ReadOnlyHint != expected.readOnly || destructive != expected.destructive {
			t.Errorf("%s: readOnly %v, destructive %v; want %v, %v",
				tool.Name, tool.Annotations.ReadOnlyHint, destructive, expected.readOnly, expected.destructive)
		}
	}
}

func TestNoToolLetsAnAgentTouchConnectionsSettingsOrWhoIsAskedToReview(t *testing.T) {
	for _, tool := range listed(t) {
		name := strings.ToLower(tool.Name)
		for _, word := range []string{"connect", "connection", "setting", "rule", "login", "app"} {
			if strings.Contains(name, word) {
				t.Errorf("%s: an agent must never manage gitd's connections or publishing settings", tool.Name)
			}
		}
		schema, _ := json.Marshal(tool.InputSchema)
		for _, word := range []string{"requested_by", "reviewer", "label"} {
			if strings.Contains(strings.ToLower(string(schema)), word) {
				t.Errorf("%s takes %q: who reviews is the platform's to say, not the agent's", tool.Name, word)
			}
		}
	}
}
