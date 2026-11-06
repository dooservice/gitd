package cli

import "github.com/spf13/cobra"

func downCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop the gitd that gitd up started, keeping its configuration, pools and workspaces",
		Long: "Stops the gitd that gitd up started and keeps it from starting with your session.\n" +
			"Its configuration, pools and workspaces are kept: gitd up starts it again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return app.down() },
	}
}
