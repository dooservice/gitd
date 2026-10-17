package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func publishCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "publish PATH BRANCH",
		Short: "Push what was committed in a workspace to a new branch",
		Long: "Push what was committed in the workspace at PATH to a new branch BRANCH, from its\n" +
			"pool and with the credential gitd keeps, and open its pull request when the\n" +
			"publishing settings or --pull-request say so. The workspace never sees the\n" +
			"credential, and the push is never forced.",
		Args: cobra.ExactArgs(2),
	}
	credentials := addCredentialFlags(command)
	var requestedBy, title, body, base, agent, model, session, task string
	var open, draft bool
	flags := command.Flags()
	flags.StringVar(&requestedBy, "requested-by", "", "who asked for the work, as their account on the host")
	flags.BoolVar(&open, "pull-request", false, "open a pull request even when the settings open them only when asked")
	flags.StringVar(&title, "title", "", "the pull request's title (default: the newest commit's subject)")
	flags.StringVar(&body, "body", "", "text that comes before what gitd checked")
	flags.StringVar(&base, "base", "", "the branch it merges into (default: the repository's default branch)")
	flags.BoolVar(&draft, "draft", false, "open it as a draft (default: as the settings say)")
	flags.StringVar(&agent, "agent", "", "the agent that did the work, for the record and the pull request")
	flags.StringVar(&model, "model", "", "the model it ran on")
	flags.StringVar(&session, "session", "", "the platform's name for the session or run")
	flags.StringVar(&task, "task-url", "", "where the task is: an issue, a ticket (https)")
	command.RunE = func(cmd *cobra.Command, args []string) error {
		given, err := credentials.proto()
		if err != nil {
			return err
		}
		request := &gitdv1.PublishRequest{Path: args[0], Branch: args[1], Credentials: given, RequestedBy: requestedBy}
		if agent != "" || model != "" || session != "" || task != "" {
			request.Provenance = &gitdv1.Provenance{Agent: agent, Model: model, Session: session, TaskUrl: task}
		}
		if open || title != "" || body != "" || base != "" || cmd.Flags().Changed("draft") {
			request.PullRequest = &gitdv1.PullRequestOptions{Open: open, Title: title, Body: body, Base: base}
			if cmd.Flags().Changed("draft") {
				request.PullRequest.Draft = &draft
			}
		}
		answer, err := app.client().Publish(cmd.Context(), request)
		if err != nil {
			return err
		}
		app.print(answer, func(out io.Writer) {
			published := append([]*gitdv1.PublishedRepository{answer.GetRepository()}, answer.GetSubmodules()...)
			for _, repository := range published {
				fmt.Fprintf(out, "%s: branch %s at %s\n", repository.GetUrl(), answer.GetBranch(), short(repository.GetCommit()))
				for _, subject := range repository.GetSubjects() {
					fmt.Fprintf(out, "  %s\n", subject)
				}
			}
			pullRequestLine(out, answer.GetPullRequest(), answer.GetPullRequestFailure())
		})
		return nil
	}
	return command
}
