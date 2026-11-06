package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func infoCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Say which gitd answers, on which git, and the disk its pools take",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			answer, err := app.client().Info(cmd.Context())
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				describeGitd(out, answer)
				fmt.Fprint(out, copyOnWrite{answer: answer}.hint())
			})
			return nil
		},
	}
}

// describeGitd says which gitd answered, on which git, what its pools and templates take,
// and how it makes workspaces, and why.
func describeGitd(out io.Writer, answer *gitdv1.InfoResponse) {
	fmt.Fprintf(out, "gitd %s on %s\n%d pools, %s; %d templates, %s\nworkspaces made by %s: %s\n",
		answer.GetVersion(), answer.GetGitVersion(), answer.GetPools(), size(answer.GetPoolsBytes()),
		answer.GetTemplates(), size(answer.GetTemplatesBytes()),
		strategyName(answer.GetStrategy()), answer.GetStrategyReason())
}
