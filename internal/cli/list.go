package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func listCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the workspaces gitd made, with their branch and what was published from each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			answer, err := app.client().List(cmd.Context())
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				if len(answer.GetWorkspaces()) == 0 {
					fmt.Fprintln(out, "no workspaces")
				}
				for _, listed := range answer.GetWorkspaces() {
					described := listed.GetDescription()
					if described == nil {
						fmt.Fprintf(out, "%s\n  cannot be described now\n", listed.GetPath())
						continue
					}
					on := "detached"
					if described.GetBranch() != "" {
						on = "on " + described.GetBranch()
					}
					changed := ""
					if described.GetModified() {
						changed = ", modified"
					}
					fmt.Fprintf(out, "%s\n  %s at %s, %s%s\n", listed.GetPath(), described.GetUrl(), short(described.GetCommit()), on, changed)
					if published := listed.GetPublished(); len(published) > 0 {
						fmt.Fprintf(out, "  published: %s\n", strings.Join(published, ", "))
					}
				}
			})
			return nil
		},
	}
}
