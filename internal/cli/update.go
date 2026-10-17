package cli

import (
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func updateCommand(app *app) *cobra.Command {
	var branch string
	var submodules bool
	command := &cobra.Command{
		Use:   "update PATH",
		Short: "Bring a workspace to the tip of its branch",
		Args:  cobra.ExactArgs(1),
	}
	command.Flags().StringVar(&branch, "branch", "", "the branch to follow (default: the one it follows)")
	command.Flags().BoolVar(&submodules, "submodules", false, "bring the submodules along")
	credentials := addCredentialFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		result, err := app.client().Update(cmd.Context(), &gitdv1.UpdateRequest{
			Path: args[0], Branch: branch, WithSubmodules: submodules, Credentials: given,
		}, app.progress())
		if err != nil {
			return err
		}
		app.print(result, func(out io.Writer) { workspaceLine(out, args[0], result) })
		return nil
	}
	return command
}
