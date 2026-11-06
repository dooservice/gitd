//go:build !windows

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dooservice/gitd/client"
	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/userservice"
)

// up writes the user's configuration when they have none and starts gitd as their service,
// or, with no manager of user services here, says how to run it in a terminal.
func (app *app) up(ctx context.Context) error {
	gitd, err := newUserGitd()
	if err != nil {
		return err
	}
	service, err := gitd.service()
	if err != nil {
		return err
	}
	manager, err := userservice.ForSystem()
	if errors.Is(err, userservice.ErrUnavailable) {
		if answer, answered := gitd.answer(ctx); answered {
			app.print(answer, func(out io.Writer) { gitd.describe(ctx, out, answer, service, inATerminal) })
			return nil
		}
		gitd.inTerminal(app.stdout, service, err)
		return errors.New("gitd was not started")
	}
	if err != nil {
		return err
	}
	if answer, answered := gitd.answer(ctx); answered && !manager.Installed() {
		return fmt.Errorf("gitd %s already answers on %s, and gitd up did not start it: stop it first",
			answer.GetVersion(), gitd.settings.Socket)
	}
	if err := manager.Start(service); err != nil {
		return err
	}
	answer, err := waitForGitd(ctx, gitd.settings.Socket)
	if err != nil {
		return fmt.Errorf("%w; its log: %s", err, manager.Log())
	}
	app.print(answer, func(out io.Writer) {
		gitd.describe(ctx, out, answer, service, running{log: manager.Log()})
	})
	return nil
}

// down stops the gitd `gitd up` started, and keeps it from starting again.
func (app *app) down() error {
	manager, err := userservice.ForSystem()
	if err != nil {
		return err
	}
	if !manager.Installed() {
		fmt.Fprintln(app.stdout, "gitd up started no gitd here")
		return nil
	}
	if err := manager.Stop(); err != nil {
		return err
	}
	places, _ := config.ForUser()
	fmt.Fprintf(app.stdout, "stopped gitd; kept its configuration, %s, and its pools and workspaces: gitd up starts it again\n",
		places.Config)
	return nil
}

// running is how the user's gitd runs: where its log is read.
type running struct {
	log string
}

// inATerminal is gitd run in a terminal, where no manager runs it.
var inATerminal = running{log: "the terminal it runs in"}

// userGitd is gitd run for the person at this terminal: its places, whether its
// configuration was written now, the configuration as read, and the GitHub CLI's path when
// it is on the PATH and logged in.
type userGitd struct {
	places   config.User
	written  bool
	settings config.Config
	gitHub   string
}

// newUserGitd writes the user's configuration unless they have one, and reads it. The
// GitHub CLI's token is read with it only when gh answers with one now; the token itself
// never reaches this process.
func newUserGitd() (userGitd, error) {
	places, ok := config.ForUser()
	if !ok {
		return userGitd{}, errors.New("no home directory to keep gitd's configuration in")
	}
	gitd := userGitd{places: places}
	if gh, err := exec.LookPath("gh"); err == nil && exec.Command(gh, "auth", "token").Run() == nil {
		gitd.gitHub = gh
	}
	first := userConfig{places: places, gitHub: gitd.gitHub != ""}
	first.git, _ = exec.LookPath("git")
	written, err := first.write()
	if err != nil {
		return userGitd{}, fmt.Errorf("writing %s: %w", places.Config, err)
	}
	gitd.written = written
	gitd.settings, err = config.Load(places.Config)
	if err != nil {
		return userGitd{}, fmt.Errorf("reading %s: %w", places.Config, err)
	}
	return gitd, nil
}

// service is gitd as the user's service. A credential read from $GH_TOKEN has the service
// read the GitHub CLI's token; one read from any other variable is refused, as a service
// started with the session would not have it.
func (gitd userGitd) service() (userservice.Service, error) {
	executable, err := os.Executable()
	if err != nil {
		return userservice.Service{}, err
	}
	service := userservice.Service{Executable: executable, Config: gitd.places.Config}
	for _, credential := range gitd.settings.Credentials {
		switch credential.TokenEnv {
		case "":
		case gitHubTokenVariable:
			if gitd.gitHub == "" {
				return service, fmt.Errorf("%s reads $%s, which gitd up fills from the GitHub CLI, and gh is not on the PATH "+
					"or not logged in: gh auth login", gitd.places.Config, gitHubTokenVariable)
			}
			service.GitHubCLI = gitd.gitHub
		default:
			return service, fmt.Errorf("%s reads $%s, which a service started with your session would not have: "+
				"give that token a token_file, or keep it in gitd with gitd connections add-token", gitd.places.Config, credential.TokenEnv)
		}
	}
	return service, nil
}

// answer is who answers on the user's socket now, if anyone does.
func (gitd userGitd) answer(ctx context.Context) (*gitdv1.InfoResponse, bool) {
	asked, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	answer, err := client.New(gitd.settings.Socket).Info(asked)
	return answer, err == nil
}

// inTerminal says how to run gitd in a terminal, where no manager can start it.
func (gitd userGitd) inTerminal(out io.Writer, service userservice.Service, why error) {
	command := shellWord(service.Executable) + " serve --config " + shellWord(service.Config)
	if service.GitHubCLI != "" {
		command = gitHubTokenVariable + `="$(` + shellWord(service.GitHubCLI) + ` auth token)" ` + command
	}
	fmt.Fprintf(out, "gitd up cannot start gitd as your service here (%v).\n"+
		"Run it in a terminal, and leave that open:\n\n  %s\n\n", why, command)
	if gitd.settings.Socket == gitd.places.Socket {
		fmt.Fprintln(out, "Other terminals find it with no --socket.")
	} else {
		fmt.Fprintf(out, "Other terminals find it with %s=%s.\n", client.SocketVariable, shellWord(gitd.settings.Socket))
	}
}

// describe says what gitd info says, where things are, and which credential GitHub is read
// with.
func (gitd userGitd) describe(ctx context.Context, out io.Writer, answer *gitdv1.InfoResponse, service userservice.Service, how running) {
	describeGitd(out, answer)
	fmt.Fprintln(out)
	configuration := gitd.places.Config + " (yours, kept)"
	if gitd.written {
		configuration = gitd.places.Config + " (written now)"
	}
	place(out, "configuration", configuration)
	place(out, "state", gitd.settings.State)
	place(out, "socket", gitd.settings.Socket)
	place(out, "workspaces", strings.Join(gitd.settings.Roots, ", "))
	place(out, "log", how.log)
	if gitd.settings.Socket != gitd.places.Socket {
		fmt.Fprintf(out, "\nThe command line finds this gitd with %s=%s.\n", client.SocketVariable, shellWord(gitd.settings.Socket))
	}
	if service.GitHubCLI != "" {
		fmt.Fprintln(out, "\nGitHub is read with the GitHub CLI's token, read as gitd starts and never written down.")
	} else if connections, err := client.New(gitd.settings.Socket).ListConnections(ctx); err == nil && len(connections.GetConnections()) == 0 {
		fmt.Fprintln(out, "\ngitd holds no credential yet. To publish to GitHub, sign in: gitd connect github")
	}
}
