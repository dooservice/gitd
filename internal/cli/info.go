package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
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
				fmt.Fprintf(out, "gitd %s on %s\n%d pools, %s\n",
					answer.GetVersion(), answer.GetGitVersion(), answer.GetPools(), size(answer.GetPoolsBytes()))
			})
			return nil
		},
	}
}
