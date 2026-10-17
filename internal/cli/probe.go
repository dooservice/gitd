package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func probeCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "probe URL",
		Short: "Say whether a repository can be read, and its branches",
		Args:  cobra.ExactArgs(1),
	}
	credentials := addCredentialFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		answer, err := app.client().Probe(cmd.Context(), &gitdv1.ProbeRequest{Url: args[0], Credentials: given})
		if err != nil {
			return err
		}
		access := strings.ToLower(strings.TrimPrefix(answer.GetAccess().String(), "ACCESS_"))
		app.print(answer, func(out io.Writer) {
			if failure := answer.GetFailure(); failure != nil {
				fmt.Fprintf(out, "%s: %s\n", access, failure.GetGitSaid())
				return
			}
			fmt.Fprintf(out, "%s, default branch %s\n", access, answer.GetDefaultBranch())
			for _, branch := range answer.GetBranches() {
				fmt.Fprintf(out, "  %s  %s\n", short(answer.GetCommits()[branch]), branch)
			}
		})
		if failure := answer.GetFailure(); failure != nil {
			return errors.New(strings.TrimPrefix(failure.GetReason().String(), "REASON_"))
		}
		return nil
	}
	return command
}
