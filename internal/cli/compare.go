package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// sideOf reads a side as the command line writes it: a workspace by its absolute path, a
// branch by its name.
func sideOf(written string) *gitdv1.Side {
	if strings.HasPrefix(written, "/") {
		return &gitdv1.Side{Of: &gitdv1.Side_Workspace{Workspace: written}}
	}
	return &gitdv1.Side{Of: &gitdv1.Side_Branch{Branch: written}}
}

// sideName is a side as people read it.
func sideName(side *gitdv1.Side) string {
	if side.GetWorkspace() != "" {
		return side.GetWorkspace()
	}
	return side.GetBranch()
}

func compareCommand(app *app) *cobra.Command {
	var url string
	var withDiff, ignored bool
	var maxBytes int64
	command := &cobra.Command{
		Use:   "compare FIRST SECOND",
		Short: "Show how two works differ: workspaces by path, branches by name",
		Long: "Show how two works of one repository differ, from FIRST to SECOND: each a workspace, by\n" +
			"its absolute path, or a branch gitd published or its pool holds, by its name, with --url\n" +
			"when neither is a workspace.",
		Example: "  gitd compare /var/lib/gitd/workspaces/agent-3 /var/lib/gitd/workspaces/agent-7 --diff\n" +
			"  gitd compare agents/fix-a agents/fix-b --url https://github.com/acme/web",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().Compare(cmd.Context(), &gitdv1.CompareRequest{
				First: sideOf(args[0]), Second: sideOf(args[1]), Url: url, WithDiff: withDiff, MaxBytes: maxBytes,
				IncludeIgnored: ignored,
			})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				changesLines(out, &gitdv1.ChangesResponse{Base: "", Files: answer.GetFiles()})
				fmt.Fprint(out, answer.GetDiff())
			})
			return nil
		},
	}
	command.Flags().StringVar(&url, "url", "", "the repository, when both are branches")
	command.Flags().BoolVar(&withDiff, "diff", false, "print the diff too")
	command.Flags().Int64Var(&maxBytes, "max-bytes", 0, "at most this many bytes of diff")
	command.Flags().BoolVar(&ignored, "ignored", false, "include files the base commit's .gitignore ignores")
	return command
}

func conflictsCommand(app *app) *cobra.Command {
	var url string
	command := &cobra.Command{
		Use:   "conflicts WORK WORK...",
		Short: "Show which works do not merge cleanly, pair by pair",
		Long: "Show which pairs of works — workspaces by path, standing for what they committed, or\n" +
			"branches by name — do not merge cleanly, and on which files, read with git merge-tree.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			request := &gitdv1.ConflictsRequest{Url: url}
			for _, written := range args {
				request.Sides = append(request.Sides, sideOf(written))
			}
			answer, err := app.client().Conflicts(cmd.Context(), request)
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { conflictLines(out, answer) })
			return nil
		},
	}
	command.Flags().StringVar(&url, "url", "", "the repository, when no work is a workspace")
	return command
}

func watchCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "watch [PATH...]",
		Short: "Print what happens to workspaces as it happens",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.client().Watch(cmd.Context(), args, func(event *gitdv1.WatchResponse) {
				app.print(event, func(out io.Writer) { eventLine(out, event) })
			})
		},
	}
}

// conflictLines are conflicts as people read them.
func conflictLines(out io.Writer, answer *gitdv1.ConflictsResponse) {
	if len(answer.GetConflicts()) == 0 {
		fmt.Fprintln(out, "every pair merges cleanly")
		return
	}
	for _, conflict := range answer.GetConflicts() {
		fmt.Fprintf(out, "%s and %s conflict on %s\n", sideName(conflict.GetFirst()), sideName(conflict.GetSecond()),
			strings.Join(conflict.GetPaths(), ", "))
	}
}

// eventLine is an event as people read it.
func eventLine(out io.Writer, event *gitdv1.WatchResponse) {
	kind := strings.ToLower(strings.TrimPrefix(event.GetKind().String(), "KIND_"))
	line := fmt.Sprintf("%s  %-9s %s", event.GetTime().AsTime().UTC().Format(time.TimeOnly), kind, event.GetPath())
	if branch := event.GetBranch(); branch != "" {
		line += "  " + branch
	}
	if detail := event.GetDetail(); detail != "" {
		line += "  " + detail
	}
	fmt.Fprintln(out, line)
}
