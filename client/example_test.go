package client_test

import (
	"context"
	"fmt"

	"github.com/dooservice/gitd/client"
	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// An agent's round trip: a workspace, the agent's work in it with plain git, its commits
// published as a branch, and the workspace removed. No call takes a credential.
func Example() {
	ctx := context.Background()
	gitd, err := client.Open(client.Options{})
	if err != nil {
		panic(err)
	}
	path := "/var/lib/gitd/workspaces/agent-1"
	workspace, err := gitd.Prepare(ctx, &gitdv1.PrepareRequest{Url: "https://github.com/acme/web", Path: path}, nil)
	if reason, said := client.Reason(err); reason != "" {
		fmt.Println("refused:", reason, said)
		return
	}
	fmt.Println(path, workspace.GetCommit(), workspace.GetStrategy())

	published, err := gitd.Publish(ctx, &gitdv1.PublishRequest{Path: path, Branch: "agents/fix-login"})
	if err != nil {
		panic(err)
	}
	fmt.Println(published.GetBranch())
	_ = gitd.Remove(ctx, path)
}

// A gitd on another machine, with the key naming the caller's tenant.
func ExampleOpen_network() {
	gitd, err := client.Open(client.Options{URL: "https://gitd.internal:9420", Key: "acme-key"})
	if err != nil {
		panic(err)
	}
	_, _ = gitd.Info(context.Background())
}
