package connections

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/secrets"
)

// fakeSignIn answers as GitHub does for a person who enters their code on the second ask.
func fakeSignIn(t *testing.T) *httptest.Server {
	t.Helper()
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		switch {
		case request.URL.Path == "/login/device/code":
			_, _ = writer.Write([]byte(`{"device_code": "dev", "user_code": "3F7A-91KD",
				"verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 5}`))
		case request.URL.Path == "/login/oauth/access_token" && request.Form.Get("grant_type") == "refresh_token":
			if request.Form.Get("refresh_token") != "ghr_1" {
				_, _ = writer.Write([]byte(`{"error": "bad_refresh_token"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"access_token": "ghu_2", "expires_in": 28800, "refresh_token": "ghr_2",
				"refresh_token_expires_in": 15897600}`))
		case request.URL.Path == "/login/oauth/access_token":
			asked++
			if asked == 1 {
				_, _ = writer.Write([]byte(`{"error": "authorization_pending"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"access_token": "ghu_1", "expires_in": 28800, "refresh_token": "ghr_1",
				"refresh_token_expires_in": 15897600}`))
		case request.URL.Path == "/user":
			_, _ = writer.Write([]byte(`{"login": "josue"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func signInStore(t *testing.T, github GitHub) (*Store, *time.Time) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "connections"), &secrets.Box{})
	if err != nil {
		t.Fatal(err)
	}
	store.UseGitHub(github)
	now := time.Unix(1_800_000_000, 0)
	store.now = func() time.Time { return now }
	store.wait = func(context.Context, time.Duration) error { return nil }
	return store, &now
}

func TestAPersonSignsInAndTheirTokenIsRenewedWithTheRenewalReplaced(t *testing.T) {
	server := fakeSignIn(t)
	store, now := signInStore(t, GitHub{APIBase: server.URL, WebBase: server.URL, LoginClientID: "Iv1.public", LoginAppSlug: "gitd"})
	ctx := context.Background()
	started, err := store.StartLogin(ctx, "")
	if err != nil || started.UserCode != "3F7A-91KD" || started.LoginID == "" {
		t.Fatalf("StartLogin = %+v, %v", started, err)
	}
	record, install, err := store.FinishLogin(ctx, started.LoginID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ActsAs != "josue" || record.URLPrefix != "https://github.com" || install != server.URL+"/apps/gitd/installations/new" {
		t.Errorf("record = %+v, install %q", record, install)
	}
	repository, _ := repo.Of("https://github.com/acme/web")
	minter := store.Credentials()[0].Minter
	if token, _ := minter.Token(ctx, repository, credentials.Write); token != "ghu_1" {
		t.Errorf("first token = %q", token)
	}
	*now = now.Add(8 * time.Hour)
	if token, err := minter.Token(ctx, repository, credentials.Write); err != nil || token != "ghu_2" {
		t.Errorf("renewed token = %q, %v", token, err)
	}
	if _, renewal, _ := store.Get(record.ID); string(renewal) != "ghr_2" {
		t.Errorf("the renewal kept is %q, not the one GitHub replaced it with", renewal)
	}
	if _, _, err := store.FinishLogin(ctx, started.LoginID); !errors.Is(err, ErrInvalid) {
		t.Errorf("a sign-in finished twice: %v", err)
	}
}

func TestSigningInIsNotOfferedWithoutGitdsPublicApp(t *testing.T) {
	store, _ := signInStore(t, GitHub{})
	if _, err := store.StartLogin(context.Background(), ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}
