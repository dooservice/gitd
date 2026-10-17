package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func auditCommand(app *app) *cobra.Command {
	var since time.Duration
	var repository string
	var limit int32
	command := &cobra.Command{
		Use:   "audit",
		Short: "Show every publish, done or refused, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			request := &gitdv1.AuditRequest{Url: repository, Limit: limit}
			if since > 0 {
				request.Since = timestamppb.New(time.Now().Add(-since))
			}
			answer, err := app.client().Audit(cmd.Context(), request)
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				for _, entry := range answer.GetEntries() {
					auditLine(out, entry)
				}
			})
			return nil
		},
	}
	command.Flags().DurationVar(&since, "since", 0, "only the publishes of this long ago or later, e.g. 24h")
	command.Flags().StringVar(&repository, "repository", "", "only the publishes of the repository at this URL")
	command.Flags().Int32Var(&limit, "limit", 100, "at most this many (up to 1000)")
	return command
}

// auditLine is one publish as people read it: when, the branch, the repository, and what
// was pushed or why it was refused.
func auditLine(out io.Writer, entry *gitdv1.AuditEntry) {
	when := entry.GetTime().AsTime().UTC().Format(time.DateTime)
	if change := entry.GetChange(); change != nil {
		action := strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(change.GetAction().String(), "ACTION_")), "_", " ")
		fmt.Fprintf(out, "%s  %s  %s  %s\n", when, action, change.GetSubject(), shownPrefix(change.GetUrlPrefix()))
		return
	}
	fmt.Fprintf(out, "%s  %s  %s  %s\n", when, entry.GetBranch(), entry.GetUrl(), entry.GetWorkspace())
	if requester := entry.GetRequestedBy(); requester != "" {
		fmt.Fprintf(out, "  requested by %s\n", requester)
	}
	if refusal := entry.GetRefusal(); refusal != nil {
		reason := strings.TrimPrefix(refusal.GetReason().String(), "REASON_")
		fmt.Fprintf(out, "  refused, %s: %s\n", reason, refusal.GetGitSaid())
		return
	}
	published := append([]*gitdv1.PublishedRepository{entry.GetRepository()}, entry.GetSubmodules()...)
	for _, pushed := range published {
		fmt.Fprintf(out, "  published %s %s: %s\n", pushed.GetUrl(), short(pushed.GetCommit()), strings.Join(pushed.GetSubjects(), "; "))
	}
	pullRequestLine(out, entry.GetPullRequest(), entry.GetPullRequestFailure())
}

// pullRequestLine says what pull request a publish opened, or why it could not.
func pullRequestLine(out io.Writer, opened *gitdv1.OpenedPullRequest, failure *gitdv1.GitFailure) {
	switch {
	case opened != nil:
		fmt.Fprintf(out, "  pull request #%d: %s\n", opened.GetNumber(), opened.GetUrl())
	case failure != nil:
		reason := strings.TrimPrefix(failure.GetReason().String(), "REASON_")
		fmt.Fprintf(out, "  no pull request, %s: %s\n", reason, failure.GetGitSaid())
	}
}
