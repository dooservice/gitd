package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/connectpage"
)

func connectCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "connect",
		Short: "Connect gitd to GitHub, with no token to make by hand",
		Long: "Connect gitd to GitHub: `github` signs you in with a code, and pull requests appear\n" +
			"as you; `github-app` creates your organisation's own App in one click, and pull\n" +
			"requests appear as its bot. Platforms do the same through the API.",
		Args: cobra.NoArgs,
	}
	command.AddCommand(connectGitHubCommand(app), connectGitHubAppCommand(app))
	return command
}

func connectGitHubCommand(app *app) *cobra.Command {
	var host string
	command := &cobra.Command{
		Use:   "github",
		Short: "Sign in to GitHub with a code; pull requests appear as you",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			started, err := app.client().StartGitHubLogin(cmd.Context(), host)
			if err != nil {
				return err
			}
			fmt.Fprintf(app.stderr, "Open %s and enter the code %s\n", started.GetVerificationUrl(), started.GetUserCode())
			finished, err := app.client().FinishGitHubLogin(cmd.Context(), started.GetLoginId())
			if err != nil {
				return err
			}
			app.print(finished, func(out io.Writer) { connected(out, finished.GetConnection(), finished.GetInstallUrl()) })
			return nil
		},
	}
	command.Flags().StringVar(&host, "host", "", "a GitHub Enterprise Server's host (default github.com)")
	return command
}

func connectGitHubAppCommand(app *app) *cobra.Command {
	var host, organization, name string
	var port int
	command := &cobra.Command{
		Use:   "github-app",
		Short: "Create your organisation's own GitHub App in one click; pull requests appear as its bot",
		Long: "Create a GitHub App owned by --org, or by you, named --name: open the address printed\n" +
			"in a browser on this machine and press \"Create GitHub App\". GitHub sends the browser\n" +
			"back here, and gitd keeps the App's key. Over SSH, forward the port first\n" +
			"(ssh -L PORT:127.0.0.1:PORT) and give it with --port.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			page, err := newManifestPage(port, connectpage.App{Name: name, Organization: organization, Host: host})
			if err != nil {
				return err
			}
			defer page.close()
			begun, err := app.client().BeginGitHubApp(cmd.Context(), &gitdv1.BeginGitHubAppRequest{
				Host: host, Organization: organization, Name: name, RedirectUrl: page.redirectURL(),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(app.stderr, "Open %s and press \"Create GitHub App\"\n", page.serve(begun))
			code, err := page.waitForCode(cmd.Context())
			if err != nil {
				return err
			}
			completed, err := app.client().CompleteGitHubApp(cmd.Context(), &gitdv1.CompleteGitHubAppRequest{
				Code: code, State: begun.GetState(),
			})
			page.finish(completed, err)
			if err != nil {
				return err
			}
			app.print(completed, func(out io.Writer) { connected(out, completed.GetConnection(), completed.GetInstallUrl()) })
			return nil
		},
	}
	command.Flags().StringVar(&organization, "org", "", "the organisation that owns the App (default: you)")
	command.Flags().StringVar(&name, "name", "", "the App's name on GitHub, e.g. acme-agents")
	command.Flags().StringVar(&host, "host", "", "a GitHub Enterprise Server's host (default github.com)")
	command.Flags().IntVar(&port, "port", 0, "the local port GitHub sends the browser back to (default: any free one)")
	_ = command.MarkFlagRequired("name")
	return command
}

// connected says what was connected and where to install it.
func connected(out io.Writer, connection *gitdv1.Connection, install string) {
	connectionLine(out, connection)
	if install != "" {
		fmt.Fprintf(out, "Install it on the repositories it may reach: %s\n", install)
	}
}
