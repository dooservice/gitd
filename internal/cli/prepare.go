package cli

import (
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func prepareCommand(app *app) *cobra.Command {
	var branch, commit string
	var submodules, fullHistory bool
	command := &cobra.Command{
		Use:   "prepare URL PATH",
		Short: "Make a workspace of a repository at PATH, in seconds",
		Long: "Make a workspace of the repository at URL in PATH, at a branch, a commit or its\n" +
			"default branch. A workspace gitd already made there is moved instead.",
		Args: cobra.ExactArgs(2),
	}
	command.Flags().StringVar(&branch, "branch", "", "check out this branch")
	command.Flags().StringVar(&commit, "commit", "", "check out this commit, detached")
	command.Flags().BoolVar(&submodules, "submodules", false, "check the submodules out too")
	command.Flags().BoolVar(&fullHistory, "full-history", false, "bring the whole history, for git log and git blame")
	command.MarkFlagsMutuallyExclusive("branch", "commit")
	credentials := addCredentialFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		request := &gitdv1.PrepareRequest{
			Url: args[0], Path: args[1], WithSubmodules: submodules, FullHistory: fullHistory, Credentials: given,
		}
		switch {
		case branch != "":
			request.Revision = &gitdv1.Revision{Target: &gitdv1.Revision_Branch{Branch: branch}}
		case commit != "":
			request.Revision = &gitdv1.Revision{Target: &gitdv1.Revision_Commit{Commit: commit}}
		}
		result, err := app.client().Prepare(cmd.Context(), request, app.progress())
		if err != nil {
			return err
		}
		app.print(result, func(out io.Writer) { workspaceLine(out, args[1], result) })
		return nil
	}
	return command
}
