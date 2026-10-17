package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func adoptCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "adopt PATH",
		Short: "Take an existing clone in, linking it to its pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.client().Adopt(cmd.Context(), &gitdv1.AdoptRequest{Path: args[0]}, app.progress())
			if err != nil {
				return err
			}
			app.print(result, func(out io.Writer) {
				fmt.Fprintf(out, "%s adopted, %s released\n", args[0], size(result.GetBytesReleased()))
			})
			return nil
		},
	}
}
