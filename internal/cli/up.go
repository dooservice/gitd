package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/dooservice/gitd/client"
	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// answerWithin is how long `gitd up` waits for the gitd it started to answer.
const answerWithin = 30 * time.Second

func upCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Run gitd for you on this machine, started with your session",
		Long: "Runs gitd for you on this machine. On Linux and macOS it writes your configuration if\n" +
			"you have none, keeping the one you have, and starts gitd as your user service\n" +
			"(systemd's user instance, launchd), started with your session and restarted when it\n" +
			"fails; the command line then finds it with no --socket. With the GitHub CLI logged in,\n" +
			"gitd reads its token each time it starts and never writes it down. On Windows it\n" +
			"installs and starts the service gitd, from an administrator's terminal.\n\n" +
			"It waits until gitd answers, and says what gitd info says, where things are, and how\n" +
			"to have workspaces made copy-on-write when they are copied.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return app.up(cmd.Context()) },
	}
}

// waitForGitd asks the gitd on socket who it is until it answers, for answerWithin at most.
func waitForGitd(ctx context.Context, socket string) (*gitdv1.InfoResponse, error) {
	gitd := client.New(socket)
	deadline := time.Now().Add(answerWithin)
	for {
		asked, cancel := context.WithTimeout(ctx, 2*time.Second)
		answer, err := gitd.Info(asked)
		cancel()
		if err == nil {
			return answer, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("gitd did not answer on %s within %s: %w", socket, answerWithin, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// place is one line of where things are, aligned with the others.
func place(out io.Writer, name, where string) {
	fmt.Fprintf(out, "%-14s %s\n", name, where)
}
