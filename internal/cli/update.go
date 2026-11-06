package cli

import (
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func updateCommand(app *app) *cobra.Command {
	var branch, merge string
	var submodules bool
	command := &cobra.Command{
		Use:   "update PATH",
		Short: "Bring a workspace to the tip of its branch, or merge another into it",
		Long: "Bring the workspace at PATH to the tip of the branch it follows, or of --branch. With\n" +
			"--merge, merge that branch of the repository into the branch the workspace is on\n" +
			"instead, with a merge commit: gitd brings it with its own credential, and nothing is\n" +
			"forced, so a branch published from the workspace still moves forward. Conflicts are\n" +
			"left marked in the workspace, to resolve and commit there.",
		Args: cobra.ExactArgs(1),
	}
	command.Flags().StringVar(&branch, "branch", "", "the branch to follow (default: the one it follows)")
	command.Flags().StringVar(&merge, "merge", "", "merge this branch into the one the workspace is on, e.g. main")
	command.MarkFlagsMutuallyExclusive("branch", "merge")
	command.Flags().BoolVar(&submodules, "submodules", false, "bring the submodules along")
	credentials := addCredentialFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		result, err := app.client().Update(cmd.Context(), &gitdv1.UpdateRequest{
			Path: args[0], Branch: branch, Merge: merge, WithSubmodules: submodules, Credentials: given,
		}, app.progress())
		if err != nil {
			return err
		}
		app.print(result, func(out io.Writer) { workspaceLine(out, args[0], result) })
		return nil
	}
	return command
}
