package client

import (
	"testing"

	"github.com/dooservice/gitd/internal/config"
)

func TestTheSocketIsTheOneGivenThenTheVariableThenTheDefault(t *testing.T) {
	t.Setenv(SocketVariable, "")
	if got := (Options{}).socket(); got != config.DefaultSocket {
		t.Errorf("with nothing set, %q; want %q", got, config.DefaultSocket)
	}
	t.Setenv(SocketVariable, "/tmp/from-variable.sock")
	if got := (Options{}).socket(); got != "/tmp/from-variable.sock" {
		t.Errorf("with the variable set, %q", got)
	}
	if got := (Options{Socket: "/tmp/given.sock"}).socket(); got != "/tmp/given.sock" {
		t.Errorf("with a socket given, %q", got)
	}
}
