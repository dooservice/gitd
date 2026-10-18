package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func tokenCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "token",
		Short: "Issue, list and revoke tokens of gitd's endpoint, for sandboxes elsewhere",
		Long: "A sandbox that cannot mount gitd's directories — a microVM, a remote sandbox, a CI runner\n" +
			"elsewhere — clones from gitd's endpoint with a token of gitd's own: it opens one\n" +
			"repository for reading and one sandbox repository for writing, for an hour unless told,\n" +
			"and is never the git host's credential. What the sandbox pushes is published with\n" +
			"`gitd publish --sandbox`.",
	}
	command.AddCommand(tokenIssueCommand(app), tokenListCommand(app), tokenRevokeCommand(app))
	return command
}

func tokenIssueCommand(app *app) *cobra.Command {
	var branch, sandbox string
	var life time.Duration
	var readOnly bool
	command := &cobra.Command{
		Use:     "issue URL",
		Short:   "Issue a token for a sandbox, with the address it clones from",
		Example: "  gitd token issue https://github.com/acme/web --for 2h\n  git clone https://x:$TOKEN@gitd.internal:9420/git/github.com/acme/web.git",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().IssueToken(cmd.Context(), &gitdv1.IssueTokenRequest{
				Url: args[0], Branch: branch, LifetimeSeconds: int64(life / time.Second), ReadOnly: readOnly, Sandbox: sandbox,
			})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				fmt.Fprintf(out, "token     %s\n", answer.GetToken())
				fmt.Fprintf(out, "clone     %s\n", answer.GetCloneUrl())
				tokenLine(out, answer.GetInfo())
				fmt.Fprintln(out, "the token is shown this once; clone with it as the password: https://x:<token>@…")
			})
			return nil
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "the branch the sandbox starts from (default: the repository's default)")
	command.Flags().DurationVar(&life, "for", 0, "how long the token lives (default 1h, at most 24h)")
	command.Flags().BoolVar(&readOnly, "read-only", false, "a token that only clones and fetches")
	command.Flags().StringVar(&sandbox, "sandbox", "", "write to this sandbox repository, from an earlier token")
	return command
}

func tokenListCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the tokens alive, never their secrets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			answer, err := app.client().ListTokens(cmd.Context())
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				if len(answer.GetTokens()) == 0 {
					fmt.Fprintln(out, "no tokens alive")
				}
				for _, token := range answer.GetTokens() {
					tokenLine(out, token)
				}
			})
			return nil
		},
	}
}

func tokenRevokeCommand(app *app) *cobra.Command {
	var removeSandbox bool
	command := &cobra.Command{
		Use:   "revoke ID",
		Short: "End a token now",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().RevokeToken(cmd.Context(), &gitdv1.RevokeTokenRequest{Id: args[0], RemoveSandbox: removeSandbox})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { fmt.Fprintf(out, "%s revoked\n", args[0]) })
			return nil
		},
	}
	command.Flags().BoolVar(&removeSandbox, "remove-sandbox", false, "remove its sandbox repository too, with whatever was pushed and not published")
	return command
}

// tokenLine is a token as people read it.
func tokenLine(out io.Writer, token *gitdv1.TokenInfo) {
	access := "reads and pushes"
	if !token.GetWritable() {
		access = "reads"
	}
	fmt.Fprintf(out, "%s  %s  %s, sandbox %s, until %s\n", token.GetId(), token.GetUrl(), access,
		token.GetSandbox(), token.GetExpiresAt().AsTime().UTC().Format(time.DateTime))
}
