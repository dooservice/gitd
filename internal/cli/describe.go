package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func describeCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "describe PATH",
		Short: "Say what a workspace is on disk, without the network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().Describe(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				on := "detached"
				if answer.GetBranch() != "" {
					on = "following " + answer.GetBranch()
				}
				changed := ""
				if answer.GetModified() {
					changed = ", modified"
				}
				fmt.Fprintf(out, "%s\n%s, %s%s, made by %s\n", answer.GetUrl(), short(answer.GetCommit()), on, changed, strategyName(answer.GetStrategy()))
				states(out, answer.GetSubmodules())
			})
			return nil
		},
	}
}
