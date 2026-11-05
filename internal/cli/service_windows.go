//go:build windows

package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/winservice"
)

// platformCommands are the commands only Windows has: `gitd service`.
func platformCommands(app *app) []*cobra.Command {
	return []*cobra.Command{serviceCommand(app)}
}

// asService serves under the service manager when it started this process, logging to
// gitd.log beside the configuration, as a service has no console.
func asService(ctx context.Context, configPath string, serve func(context.Context) error) error {
	if !winservice.Running() {
		return serve(ctx)
	}
	logFile, err := os.OpenFile(filepath.Join(filepath.Dir(configPath), "gitd.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		defer logFile.Close()
		slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	}
	return winservice.Run(serve)
}

func serviceCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "service",
		Short: "Run gitd as a Windows service (install, uninstall, start, stop, status)",
		Args:  cobra.NoArgs,
	}
	configPath := config.DefaultPath
	var member string
	install := &cobra.Command{
		Use:   "install",
		Short: "Install gitd as a service started with Windows, and let --user use it",
		Long: "Installs this gitd as the service gitd, run as LocalSystem, started with Windows and\n" +
			"restarted when it fails. It creates the local group gitd, adds --user to it (you, by\n" +
			"default), and gives the group what it needs: the socket, reading the pools and templates,\n" +
			"and changing the workspaces. Run it from an administrator's terminal.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			settings, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("reading %s: %w", configPath, err)
			}
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			if member == "" {
				if current, err := user.Current(); err == nil {
					member = current.Username
				}
			}
			absolute, err := filepath.Abs(configPath)
			if err != nil {
				return err
			}
			if err := winservice.Install(executable, absolute, member, settings.State, settings.Roots); err != nil {
				return err
			}
			fmt.Fprintf(app.stdout, "installed the service %s; %s may use it once signed in again.\nStart it with: gitd service start\n", winservice.Name, member)
			return nil
		},
	}
	install.Flags().StringVar(&configPath, "config", config.DefaultPath, "configuration file the service reads")
	install.Flags().StringVar(&member, "user", "", "the account to add to the group gitd (default: you)")
	uninstall := &cobra.Command{
		Use: "uninstall", Short: "Stop and remove the service; workspaces and pools are kept", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return winservice.Uninstall() },
	}
	start := &cobra.Command{
		Use: "start", Short: "Start the service", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return winservice.Start() },
	}
	stop := &cobra.Command{
		Use: "stop", Short: "Stop the service", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return winservice.Stop() },
	}
	status := &cobra.Command{
		Use: "status", Short: "Say whether the service is installed and running", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			state, err := winservice.State()
			if err != nil {
				return err
			}
			fmt.Fprintln(app.stdout, state)
			return nil
		},
	}
	command.AddCommand(install, uninstall, start, stop, status)
	return command
}
