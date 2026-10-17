package workspace

import (
	"context"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

func TestASubmoduleMustStayInsideTheWorkspace(t *testing.T) {
	for _, valid := range []submodule{
		{name: "web", path: "web"},
		{name: "vendor/web", path: "vendor/web"},
	} {
		if !valid.valid() {
			t.Errorf("refused %+v", valid)
		}
	}
	for _, invalid := range []submodule{
		{name: "../../../pools/github.com/acme/other.git", path: "sub"},
		{name: "web", path: "../x"},
		{name: "web", path: "/etc/x"},
		{name: "web", path: ""},
		{name: "web", path: "a/.git/hooks"},
		{name: `..\..\x`, path: "web"},
		{name: "/abs", path: "web"},
	} {
		if invalid.valid() {
			t.Errorf("accepted %+v", invalid)
		}
	}
}

func TestACheckoutSaysWhichCommit(t *testing.T) {
	_, err := (&Workspaces{}).Checkout(context.Background(), &gitdv1.CheckoutRequest{Path: "/w"}, nil)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.BadAddress {
		t.Errorf("Checkout without a commit = %v", err)
	}
}
