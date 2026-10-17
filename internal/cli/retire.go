package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

func retireCommand(app *app) *cobra.Command {
	var unused time.Duration
	command := &cobra.Command{
		Use:   "retire",
		Short: "Remove now the pools nobody used for a while",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			answer, err := app.client().Retire(cmd.Context(), int64(unused.Seconds()))
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				fmt.Fprintf(out, "%d pools retired, %s freed\n", answer.GetRetired(), size(answer.GetFreedBytes()))
			})
			return nil
		},
	}
	command.Flags().DurationVar(&unused, "unused", 30*24*time.Hour, "retire pools nobody used for this long (at least 24h)")
	return command
}
