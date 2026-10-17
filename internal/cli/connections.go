package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func connectionsCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "connections",
		Short: "List, add and remove the credentials gitd keeps",
		Long: "The credentials gitd keeps besides those of gitd.toml, made through the API: tokens,\n" +
			"GitHub Apps and GitHub sign-ins. No secret is ever shown. To connect GitHub, see\n" +
			"`gitd connect`.",
		Args: cobra.NoArgs,
	}
	command.AddCommand(listConnectionsCommand(app), removeConnectionCommand(app),
		addTokenCommand(app), addGitHubAppCommand(app))
	return command
}

func listConnectionsCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the connections, never a secret",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			answer, err := app.client().ListConnections(cmd.Context())
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				for _, connection := range answer.GetConnections() {
					connectionLine(out, connection)
				}
			})
			return nil
		},
	}
}

func removeConnectionCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "remove ID",
		Short: "Forget a connection made through the API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.client().RemoveConnection(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(app.stdout, "removed %s\n", args[0])
			return nil
		},
	}
}

func addTokenCommand(app *app) *cobra.Command {
	var tokenEnv, user string
	command := &cobra.Command{
		Use:   "add-token PREFIX",
		Short: "Keep a token for the repositories under PREFIX, on any host",
		Long: "Keep a token for the repositories whose address starts with PREFIX, which names the\n" +
			"host, e.g. https://gitlab.example.com/acme. The token is read from an environment\n" +
			"variable, never given on the command line.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token := strings.TrimSpace(os.Getenv(tokenEnv))
			if token == "" {
				return fmt.Errorf("$%s is empty", tokenEnv)
			}
			answer, err := app.client().AddToken(cmd.Context(), &gitdv1.AddTokenRequest{UrlPrefix: args[0], Token: token, User: user})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { connectionLine(out, answer.GetConnection()) })
			return nil
		},
	}
	command.Flags().StringVar(&tokenEnv, "token-env", "", "read the token from this environment variable")
	command.Flags().StringVar(&user, "user", "", "the user name the token goes with (default: the host's)")
	_ = command.MarkFlagRequired("token-env")
	return command
}

func addGitHubAppCommand(app *app) *cobra.Command {
	var host, keyFile, prefix string
	var appID int64
	command := &cobra.Command{
		Use:   "add-github-app",
		Short: "Keep a GitHub App you already have, by its ID and private key",
		Long: "Keep a GitHub App by its ID and the file of its private key, once GitHub has taken\n" +
			"the key as the App's. To create one in a click instead, see `gitd connect github-app`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, err := os.ReadFile(keyFile)
			if err != nil {
				return err
			}
			answer, err := app.client().AddGitHubApp(cmd.Context(), &gitdv1.AddGitHubAppRequest{
				Host: host, AppId: appID, PrivateKey: string(key), UrlPrefix: prefix,
			})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { connectionLine(out, answer.GetConnection()) })
			return nil
		},
	}
	command.Flags().Int64Var(&appID, "app-id", 0, "the App's ID")
	command.Flags().StringVar(&keyFile, "private-key-file", "", "the file of the App's private key, in PEM")
	command.Flags().StringVar(&host, "host", "", "a GitHub Enterprise Server's host (default github.com)")
	command.Flags().StringVar(&prefix, "prefix", "", "the repositories it opens (default every one on the host)")
	_ = command.MarkFlagRequired("app-id")
	_ = command.MarkFlagRequired("private-key-file")
	return command
}

// connectionLine is one connection as people read it.
func connectionLine(out io.Writer, connection *gitdv1.Connection) {
	kind := strings.ToLower(strings.TrimPrefix(connection.GetKind().String(), "KIND_"))
	line := fmt.Sprintf("%s  %s  %s", connection.GetId(), kind, connection.GetUrlPrefix())
	if connection.GetActsAs() != "" {
		line += "  as " + connection.GetActsAs()
	}
	if expires := connection.GetExpires(); expires != nil {
		line += "  until " + expires.AsTime().UTC().Format(time.DateOnly)
	}
	if connection.GetFromFile() {
		line += "  (gitd.toml)"
	}
	fmt.Fprintln(out, line)
}
