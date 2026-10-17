package cli

import (
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func submodulesCommand(app *app) *cobra.Command {
	var only string
	command := &cobra.Command{
		Use:   "submodules PATH",
		Short: "Check out a workspace's submodules, each from its own pool",
		Args:  cobra.ExactArgs(1),
	}
	command.Flags().StringVar(&only, "only", "", "only the submodule of this repository")
	credentials := addCredentialFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		result, err := app.client().SyncSubmodules(cmd.Context(), &gitdv1.SyncSubmodulesRequest{
			Path: args[0], OnlyUrl: only, Credentials: given,
		}, app.progress())
		if err != nil {
			return err
		}
		app.print(result, func(out io.Writer) {
			states(out, result.GetSubmodules())
			pending(out, result.GetPendingSubmodules())
		})
		return nil
	}
	return command
}
