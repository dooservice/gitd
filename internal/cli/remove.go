package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func removeCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "remove PATH",
		Short: "Delete a workspace; its pool stays",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.client().Remove(cmd.Context(), args[0]); err != nil {
				return err
			}
			app.print(&gitdv1.RemoveResponse{}, func(out io.Writer) { fmt.Fprintf(out, "%s removed\n", args[0]) })
			return nil
		},
	}
}
