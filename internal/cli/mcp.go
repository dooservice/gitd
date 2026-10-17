package cli

import (
	"github.com/spf13/cobra"

	"github.com/dooservice/gitd/internal/mcpserver"
)

func mcpCommand(app *app, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Offer gitd to AI agents as MCP tools, over stdio",
		Long: "Answer the Model Context Protocol on stdin and stdout, so an AI agent can prepare\n" +
			"workspaces, describe them and publish branches. No tool takes a credential: gitd\n" +
			"reads and pushes with the ones it keeps, and the agent never sees them.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mcpserver.Serve(cmd.Context(), app.client(), version)
		},
	}
}
