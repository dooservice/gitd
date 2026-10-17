package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func sealCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "seal PATH NAME",
		Short: "Seal what was installed in a workspace as a warm layer for the next ones",
		Long: "Seal the own layer of the overlay workspace at PATH — what installing its dependencies\n" +
			"wrote — as the warm layer NAME of its repository. Workspaces made later at the same\n" +
			"commit with `gitd prepare --warm NAME` hold it from the start. The layer is moved, not\n" +
			"copied, so the workspace is removed. Sealing NAME again replaces it, once no workspace\n" +
			"stands on it.",
		Example: "  gitd prepare https://github.com/acme/web /var/lib/gitd/workspaces/install\n" +
			"  (cd /var/lib/gitd/workspaces/install && npm ci)\n" +
			"  gitd seal /var/lib/gitd/workspaces/install deps\n" +
			"  gitd prepare https://github.com/acme/web /var/lib/gitd/workspaces/agent-1 --warm deps",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().Seal(cmd.Context(), &gitdv1.SealRequest{Path: args[0], Name: args[1]})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { warmLine(out, answer.GetWarm()) })
			return nil
		},
	}
}

func warmCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "warm URL",
		Short: "List the warm layers of a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().ListWarm(cmd.Context(), &gitdv1.ListWarmRequest{Url: args[0]})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				if len(answer.GetWarm()) == 0 {
					fmt.Fprintln(out, "no warm layers")
				}
				for _, warm := range answer.GetWarm() {
					warmLine(out, warm)
				}
			})
			return nil
		},
	}
}

// warmLine is a warm layer as people read it.
func warmLine(out io.Writer, warm *gitdv1.WarmLayer) {
	fmt.Fprintf(out, "%s at %s, sealed %s from %s: %d files changed git sees, %d workspaces on it\n",
		warm.GetName(), short(warm.GetCommit()), time.Unix(warm.GetSealedAt(), 0).UTC().Format(time.DateTime),
		warm.GetSealedFrom(), warm.GetChangedFiles(), warm.GetUsers())
}
