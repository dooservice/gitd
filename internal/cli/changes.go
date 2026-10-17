package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// kindMarks are how a change reads in a list, as git status writes them.
var kindMarks = map[gitdv1.ChangedFile_Kind]string{
	gitdv1.ChangedFile_KIND_ADDED:    "A",
	gitdv1.ChangedFile_KIND_MODIFIED: "M",
	gitdv1.ChangedFile_KIND_DELETED:  "D",
}

func changesCommand(app *app) *cobra.Command {
	var ignored bool
	command := &cobra.Command{
		Use:   "changes PATH",
		Short: "List what was changed in a workspace since gitd put it on its commit",
		Long: "List each file added (A), modified (M) or deleted (D) in the workspace at PATH since gitd\n" +
			"last put it on a commit — committed or not — with its lines. On an overlay workspace it\n" +
			"is read from the agent's own layer, and no git runs in the workspace.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().Changes(cmd.Context(), &gitdv1.ChangesRequest{Path: args[0], IncludeIgnored: ignored})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { changesLines(out, answer) })
			return nil
		},
	}
	command.Flags().BoolVar(&ignored, "ignored", false, "include files the base commit's .gitignore ignores")
	return command
}

func diffCommand(app *app) *cobra.Command {
	var ignored bool
	var maxBytes int64
	command := &cobra.Command{
		Use:   "diff PATH [FILE...]",
		Short: "Show the diff of what was changed in a workspace",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer, err := app.client().Diff(cmd.Context(), &gitdv1.DiffRequest{
				Path: args[0], Files: args[1:], MaxBytes: maxBytes, IncludeIgnored: ignored,
			})
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				fmt.Fprint(out, answer.GetDiff())
				if answer.GetTruncated() {
					fmt.Fprintln(app.stderr, "gitd: the diff was cut short; ask for fewer files or --max-bytes")
				}
			})
			return nil
		},
	}
	command.Flags().BoolVar(&ignored, "ignored", false, "include files the base commit's .gitignore ignores")
	command.Flags().Int64Var(&maxBytes, "max-bytes", 0, "at most this many bytes of diff (default 512 KiB, at most 8 MiB)")
	return command
}

// changesLines are a workspace's changes as people read them.
func changesLines(out io.Writer, answer *gitdv1.ChangesResponse) {
	read := "git in the workspace"
	if answer.GetFromLayer() {
		read = "the agent's layer"
	}
	if answer.GetBase() != "" {
		fmt.Fprintf(out, "since %s, read from %s\n", short(answer.GetBase()), read)
	}
	if len(answer.GetFiles()) == 0 {
		fmt.Fprintln(out, "  nothing changed")
		return
	}
	var added, removed int32
	for _, file := range answer.GetFiles() {
		lines := fmt.Sprintf("+%d -%d", file.GetAddedLines(), file.GetRemovedLines())
		if file.GetBinary() {
			lines = "binary"
		}
		fmt.Fprintf(out, "  %s  %-12s %s\n", kindMarks[file.GetKind()], lines, file.GetPath())
		added, removed = added+file.GetAddedLines(), removed+file.GetRemovedLines()
	}
	fmt.Fprintf(out, "%d %s, +%d -%d\n", len(answer.GetFiles()), plural(len(answer.GetFiles()), "file"), added, removed)
}

func plural(count int, noun string) string {
	if count == 1 {
		return noun
	}
	return strings.TrimSuffix(noun, "s") + "s"
}
