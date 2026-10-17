package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func publishCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "publish PATH BRANCH",
		Short: "Push what was committed in a workspace to a new branch",
		Long: "Push what was committed in the workspace at PATH to a new branch BRANCH, from its\n" +
			"pool and with the token gitd keeps, so a pull request can be opened from it. The\n" +
			"workspace never sees the token, and the push is never forced.",
		Args: cobra.ExactArgs(2),
	}
	credentials := addCredentialFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		answer, err := app.client().Publish(cmd.Context(), &gitdv1.PublishRequest{
			Path: args[0], Branch: args[1], Credentials: given,
		})
		if err != nil {
			return err
		}
		app.print(answer, func(out io.Writer) {
			published := append([]*gitdv1.PublishedRepository{answer.GetRepository()}, answer.GetSubmodules()...)
			for _, repository := range published {
				fmt.Fprintf(out, "%s: branch %s at %s\n", repository.GetUrl(), answer.GetBranch(), short(repository.GetCommit()))
				for _, subject := range repository.GetSubjects() {
					fmt.Fprintf(out, "  %s\n", subject)
				}
			}
		})
		return nil
	}
	return command
}
