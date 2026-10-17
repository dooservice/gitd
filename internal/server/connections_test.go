package server

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/audit"
	"github.com/dooservice/gitd/internal/connections"
	"github.com/dooservice/gitd/internal/secrets"
)

func connectionsServer(t *testing.T) *Server {
	t.Helper()
	directory := t.TempDir()
	store, err := connections.Open(filepath.Join(directory, "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	auditLog, err := audit.New(filepath.Join(directory, "audit"), 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Connections: store, AuditLog: auditLog,
		FileConnections: []*gitdv1.Connection{{Id: "file-1", Kind: gitdv1.Connection_KIND_TOKEN, FromFile: true}}}
}

func TestATokenAddedThroughTheAPIIsListedWithoutItsSecretAndRecorded(t *testing.T) {
	server := connectionsServer(t)
	handlers := &connectionHandlers{server: server}
	ctx := context.Background()
	added, err := handlers.AddToken(ctx, connect.NewRequest(&gitdv1.AddTokenRequest{
		UrlPrefix: "https://gitlab.example.com/acme", Token: "glpat-secret",
	}))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := handlers.ListConnections(ctx, connect.NewRequest(&gitdv1.ListConnectionsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Msg.GetConnections()) != 2 || listed.Msg.GetConnections()[0].GetId() != "file-1" {
		t.Fatalf("listed = %v", listed.Msg)
	}
	text, _ := protojson.Marshal(listed.Msg)
	if strings.Contains(string(text), "glpat-secret") {
		t.Error("the list shows a secret")
	}
	if _, err := handlers.RemoveConnection(ctx, connect.NewRequest(&gitdv1.RemoveConnectionRequest{
		Id: added.Msg.GetConnection().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	entries, err := server.AuditLog.Read(audit.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].GetChange().GetAction() != gitdv1.ConfigurationChange_ACTION_CONNECTION_REMOVED ||
		entries[1].GetChange().GetUrlPrefix() != "https://gitlab.example.com/acme" {
		t.Errorf("audit = %v", entries)
	}
}

func TestAConnectionThatCannotBeChangedIsRefusedWithItsCode(t *testing.T) {
	handlers := &connectionHandlers{server: connectionsServer(t)}
	ctx := context.Background()
	for code, call := range map[connect.Code]func() error{
		connect.CodeInvalidArgument: func() error {
			_, err := handlers.AddToken(ctx, connect.NewRequest(&gitdv1.AddTokenRequest{UrlPrefix: "acme", Token: "t"}))
			return err
		},
		connect.CodeFailedPrecondition: func() error {
			_, err := handlers.RemoveConnection(ctx, connect.NewRequest(&gitdv1.RemoveConnectionRequest{Id: "file-1"}))
			return err
		},
		connect.CodeNotFound: func() error {
			_, err := handlers.RemoveConnection(ctx, connect.NewRequest(&gitdv1.RemoveConnectionRequest{Id: "token-0"}))
			return err
		},
	} {
		var refused *connect.Error
		if err := call(); !errors.As(err, &refused) || refused.Code() != code {
			t.Errorf("want %v, got %v", code, err)
		}
	}
}

func TestCreatingAGitHubAppStartsThroughTheAPIAndAStateIsNeverGuessed(t *testing.T) {
	handlers := &connectionHandlers{server: connectionsServer(t)}
	ctx := context.Background()
	begun, err := handlers.BeginGitHubApp(ctx, connect.NewRequest(&gitdv1.BeginGitHubAppRequest{
		Organization: "acme", Name: "acme-agents", RedirectUrl: "https://platform.example.com/github/created",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if begun.Msg.GetState() == "" || !strings.Contains(begun.Msg.GetFormAction(), "/organizations/acme/settings/apps/new") {
		t.Errorf("begun = %v", begun.Msg)
	}
	var refused *connect.Error
	if _, err := handlers.CompleteGitHubApp(ctx, connect.NewRequest(&gitdv1.CompleteGitHubAppRequest{
		Code: "code", State: "guessed",
	})); !errors.As(err, &refused) || refused.Code() != connect.CodeInvalidArgument {
		t.Errorf("a guessed state: %v", err)
	}
}

func TestSigningInWithoutGitdsPublicAppIsRefusedPlainly(t *testing.T) {
	handlers := &connectionHandlers{server: connectionsServer(t)}
	var refused *connect.Error
	if _, err := handlers.StartGitHubLogin(context.Background(), connect.NewRequest(&gitdv1.StartGitHubLoginRequest{})); !errors.As(err, &refused) ||
		refused.Code() != connect.CodeFailedPrecondition {
		t.Errorf("err = %v", err)
	}
}
