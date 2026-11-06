//go:build windows

package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/dooservice/gitd/client"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/winservice"
)

// up runs gitd as the service gitd, installing it unless it is and starting it unless it
// runs. Windows has no manager of user services gitd uses; the service, started with
// Windows, serves every member of the group gitd, and installing or starting it takes an
// administrator, once.
func (app *app) up(ctx context.Context) error {
	settings, err := config.Load(config.DefaultPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", config.DefaultPath, err)
	}
	asked, cancel := context.WithTimeout(ctx, 2*time.Second)
	answer, err := client.New(settings.Socket).Info(asked)
	cancel()
	log := filepath.Join(filepath.Dir(config.DefaultPath), "gitd.log")
	if err != nil {
		if err := app.startService(); err != nil {
			return err
		}
		if answer, err = waitForGitd(ctx, settings.Socket); err != nil {
			return fmt.Errorf("%w; its log: %s", err, log)
		}
	}
	app.print(answer, func(out io.Writer) {
		describeGitd(out, answer)
		fmt.Fprintln(out)
		place(out, "configuration", config.DefaultPath)
		place(out, "state", settings.State)
		place(out, "socket", settings.Socket)
		place(out, "workspaces", strings.Join(settings.Roots, ", "))
		place(out, "log", log)
		fmt.Fprintf(out, "\ngitd runs as the Windows service %s, for every member of the group %s.\n", winservice.Name, winservice.Group)
	})
	return nil
}

// startService installs the service unless it is, and starts it unless it runs.
func (app *app) startService() error {
	state, err := winservice.State()
	if err != nil {
		return fmt.Errorf("%w; gitd runs on Windows as a service, which an administrator installs once: "+
			"run gitd up from an administrator's terminal", err)
	}
	if state == "not installed" {
		if err := installService(app.stdout, config.DefaultPath, ""); err != nil {
			return err
		}
	} else if state == "running" {
		return nil
	}
	return winservice.Start()
}

// down stops and removes the service gitd, keeping its configuration, pools and workspaces.
func (app *app) down() error {
	state, err := winservice.State()
	if err != nil {
		return fmt.Errorf("%w; run gitd down from an administrator's terminal", err)
	}
	if state == "not installed" {
		fmt.Fprintf(app.stdout, "the service %s is not installed\n", winservice.Name)
		return nil
	}
	if err := winservice.Uninstall(); err != nil {
		return err
	}
	fmt.Fprintf(app.stdout, "stopped and removed the service %s; kept its configuration, %s, and its pools and workspaces: "+
		"gitd up starts it again\n", winservice.Name, config.DefaultPath)
	return nil
}
