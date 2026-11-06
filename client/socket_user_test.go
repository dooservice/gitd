//go:build !windows

package client

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/config"
)

func TestTheUsersOwnGitdIsCalledWhenItsSocketIsThere(t *testing.T) {
	home, err := os.MkdirTemp("/tmp", "gitd-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv(SocketVariable, "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	user, _ := config.ForUser()
	if err := os.MkdirAll(filepath.Dir(user.Socket), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user.Socket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (Options{}).socket(); got != config.DefaultSocket {
		t.Errorf("with a plain file where the socket goes, %q; want %q", got, config.DefaultSocket)
	}
	if err := os.Remove(user.Socket); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", user.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if got := (Options{}).socket(); got != user.Socket {
		t.Errorf("with the user's gitd answering, %q; want %q", got, user.Socket)
	}
	t.Setenv(SocketVariable, "/tmp/from-variable.sock")
	if got := (Options{}).socket(); got != "/tmp/from-variable.sock" {
		t.Errorf("with the variable set, %q", got)
	}
}
