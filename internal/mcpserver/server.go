// Package mcpserver offers gitd to AI agents as Model Context Protocol tools, over stdio.
//
// No tool takes a credential. An agent asks for a workspace, works in it with plain git,
// and asks for its commits to be published as a branch; gitd reads and pushes with the
// credentials it keeps, so the token never enters the agent's context, its sandbox or its
// logs. Workspaces can only be made under gitd's roots.
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dooservice/gitd/internal/client"
)

// Serve answers MCP on stdin and stdout until ctx ends or the client goes away, calling
// the gitd behind gitd.
func Serve(ctx context.Context, gitd *client.Client, version string) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "gitd", Title: "gitd", Version: version}, &mcp.ServerOptions{
		Instructions: "Workspaces are git checkouts made in seconds from a shared pool. Prepare one, " +
			"work in it with git (commit as usual), then publish_branch to push your commits to a new " +
			"branch for review. You never handle credentials: gitd holds them.",
	})
	tools := &tools{gitd: gitd}
	tools.register(server)
	return server.Run(ctx, &mcp.StdioTransport{})
}
