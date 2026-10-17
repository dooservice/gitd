package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/daemon"
)

// addServe adds `gitd serve`, which is also what `gitd` alone does, and returns where the
// configuration path is read into.
func addServe(root *cobra.Command, app *app, version string) *string {
	configPath := root.Flags().String("config", config.DefaultPath, "configuration file")
	command := &cobra.Command{
		Use:   "serve",
		Short: "Answer on the socket (what gitd alone does)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), *configPath, app.socket, version)
		},
	}
	command.Flags().AddFlag(root.Flags().Lookup("config"))
	root.AddCommand(command)
	return configPath
}

func serve(ctx context.Context, configPath, socket, version string) error {
	return daemon.Run(ctx, configPath, socket, version)
}
