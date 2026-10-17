// Package cli is gitd's command line. `gitd` alone serves; every other command calls the
// gitd that serves, on its socket.
//
// Credentials are best kept by gitd itself (`[[credentials]]` in gitd.toml). A command can
// still bring one, from an environment variable or the path of a key, never on its own
// command line where `ps` would show it.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dooservice/gitd/internal/client"
	"github.com/dooservice/gitd/internal/config"
)

// app is what every command shares: where gitd answers, how to print, and where.
type app struct {
	socket         string
	asJSON         bool
	stdout, stderr io.Writer
	// interactive is whether stderr is a terminal, the only place progress is drawn;
	// drawn is whether a progress line is there to be cleared.
	interactive bool
	drawn       bool
}

// client is the gitd to call: the socket given, then $GITD_SOCKET, then the default.
func (app *app) client() *client.Client {
	socket := app.socket
	if socket == "" {
		socket = os.Getenv("GITD_SOCKET")
	}
	if socket == "" {
		socket = config.DefaultSocket
	}
	return client.New(socket)
}

// Execute runs the command line in args and returns the process's exit code.
func Execute(ctx context.Context, args []string, version string) int {
	app := &app{stdout: os.Stdout, stderr: os.Stderr, interactive: terminal(os.Stderr)}
	root := newRoot(app, version)
	root.SetArgs(compatible(args))
	root.SetOut(app.stdout)
	root.SetErr(app.stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		app.clearProgress()
		fmt.Fprintf(app.stderr, "gitd: %s\n", describe(err))
		return 1
	}
	return 0
}

func newRoot(app *app, version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "gitd",
		Short: "Instant, secretless git checkouts for agent sandboxes",
		Long: "gitd keeps one copy of each git repository and makes workspaces that borrow from it,\n" +
			"in seconds. Whoever works in a workspace never sees the credential: gitd reads and\n" +
			"publishes with it. `gitd` alone serves; the other commands call it.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.PersistentFlags().StringVar(&app.socket, "socket", "", "gitd's Unix socket (default $GITD_SOCKET, then "+config.DefaultSocket+")")
	root.PersistentFlags().BoolVar(&app.asJSON, "json", false, "print the answer as JSON")

	configPath := addServe(root, app, version)
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return serve(cmd.Context(), *configPath, app.socket, version)
	}

	root.AddGroup(
		&cobra.Group{ID: "workspaces", Title: "Workspaces:"},
		&cobra.Group{ID: "repositories", Title: "Repositories, the server and agents:"},
	)
	for _, command := range []*cobra.Command{
		prepareCommand(app), updateCommand(app), checkoutCommand(app), submodulesCommand(app),
		describeCommand(app), publishCommand(app), removeCommand(app), adoptCommand(app),
	} {
		command.GroupID = "workspaces"
		root.AddCommand(command)
	}
	for _, command := range []*cobra.Command{probeCommand(app), infoCommand(app), retireCommand(app), mcpCommand(app, version)} {
		command.GroupID = "repositories"
		root.AddCommand(command)
	}
	return root
}

// compatible writes the single-dash long flags gitd took before its command line, as
// `-config` in a systemd unit, the way the command line reads them: `--config`.
func compatible(args []string) []string {
	out := make([]string, len(args))
	for index, arg := range args {
		name, _, _ := strings.Cut(strings.TrimPrefix(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(name) > 1 {
			arg = "-" + arg
		}
		out[index] = arg
	}
	return out
}

// describe is how a failure reads: gitd's reason and what git said, when it is one of
// gitd's refusals.
func describe(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if reason, said := client.Reason(err); reason != "" {
		return strings.TrimPrefix(reason, "REASON_") + ": " + said
	}
	return err.Error()
}
