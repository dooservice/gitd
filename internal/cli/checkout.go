package cli

import (
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func checkoutCommand(app *app) *cobra.Command {
	var submodules bool
	command := &cobra.Command{
		Use:   "checkout PATH COMMIT",
		Short: "Move a workspace to a commit, as a rollback does",
		Args:  cobra.ExactArgs(2),
	}
	command.Flags().BoolVar(&submodules, "submodules", false, "bring the submodules along")
	credentials := addCredentialFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		result, err := app.client().Checkout(cmd.Context(), &gitdv1.CheckoutRequest{
			Path: args[0], Commit: args[1], WithSubmodules: submodules, Credentials: given,
		}, app.progress())
		if err != nil {
			return err
		}
		app.print(result, func(out io.Writer) { workspaceLine(out, args[0], result) })
		return nil
	}
	return command
}
