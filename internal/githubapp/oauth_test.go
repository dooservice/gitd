package githubapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPersonSignsInWithACodeAndTheirTokensAreRenewed(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil || request.Form.Get("client_id") != "Iv1.public" ||
			request.Header.Get("Accept") != "application/json" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		switch request.URL.Path {
		case "/login/device/code":
			_, _ = writer.Write([]byte(`{"device_code": "dev", "user_code": "3F7A-91KD",
				"verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 5}`))
		case "/login/oauth/access_token":
			if request.Form.Get("grant_type") == "refresh_token" && request.Form.Get("refresh_token") == "ghr_1" {
				_, _ = writer.Write([]byte(`{"access_token": "ghu_2", "expires_in": 28800, "refresh_token": "ghr_2",
					"refresh_token_expires_in": 15897600}`))
				return
			}
			polls++
			switch polls {
			case 1:
				_, _ = writer.Write([]byte(`{"error": "authorization_pending"}`))
			case 2:
				_, _ = writer.Write([]byte(`{"error": "slow_down"}`))
			default:
				_, _ = writer.Write([]byte(`{"access_token": "ghu_1", "expires_in": 28800, "refresh_token": "ghr_1",
					"refresh_token_expires_in": 15897600}`))
			}
		}
	}))
	defer server.Close()
	oauth := NewOAuth(server.URL, "Iv1.public")
	ctx := context.Background()
	code, err := oauth.Start(ctx)
	if err != nil || code.UserCode != "3F7A-91KD" || code.Interval != 5 {
		t.Fatalf("Start = %+v, %v", code, err)
	}
	if _, err := oauth.Poll(ctx, code.DeviceCode); !errors.Is(err, ErrPending) {
		t.Errorf("first poll: %v", err)
	}
	if _, err := oauth.Poll(ctx, code.DeviceCode); !errors.Is(err, ErrSlowDown) {
		t.Errorf("second poll: %v", err)
	}
	tokens, err := oauth.Poll(ctx, code.DeviceCode)
	if err != nil || tokens.AccessToken != "ghu_1" || tokens.RefreshToken != "ghr_1" {
		t.Fatalf("third poll = %+v, %v", tokens, err)
	}
	renewed, err := oauth.Renew(ctx, tokens.RefreshToken)
	if err != nil || renewed.AccessToken != "ghu_2" || renewed.RefreshToken != "ghr_2" {
		t.Errorf("Renew = %+v, %v", renewed, err)
	}
}

func TestACodeRefusedOrExpiredSaysWhy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"error": "access_denied", "error_description": "The user has denied your application access."}`))
	}))
	defer server.Close()
	_, err := NewOAuth(server.URL, "Iv1.public").Poll(context.Background(), "dev")
	if err == nil || errors.Is(err, ErrPending) || errors.Is(err, ErrSlowDown) {
		t.Errorf("err = %v", err)
	}
}

func TestATokenSaysWhoseItIs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user" || request.Header.Get("Authorization") != "Bearer ghu_1" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"login": "josue"}`))
	}))
	defer server.Close()
	if login, err := NewAPI(server.URL, "gitd/test").User(context.Background(), "ghu_1"); err != nil || login != "josue" {
		t.Errorf("User = %q, %v", login, err)
	}
}
