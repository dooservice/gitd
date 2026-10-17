package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// presetsByName and openingsByName are how the command line names presets and ways of
// opening pull requests.
var (
	presetsByName  = map[string]gitdv1.Preset{"": gitdv1.Preset_PRESET_UNSPECIFIED, "recommended": gitdv1.Preset_PRESET_RECOMMENDED}
	openingsByName = map[string]gitdv1.PullRequestSettings_Open{
		"when-asked": gitdv1.PullRequestSettings_OPEN_WHEN_ASKED, "always": gitdv1.PullRequestSettings_OPEN_ALWAYS,
	}
)

func publishingCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "publishing",
		Short: "Show and set the rules publishing follows, per repository prefix",
		Long: "The rules publishing follows for the repositories under a prefix: a preset, paths a\n" +
			"publish may not change or delete, how many files it may change, the branch prefix,\n" +
			"and how pull requests are opened. Every setting whose prefix opens a repository\n" +
			"applies, and a narrower one never loosens a broader one.",
		Args: cobra.NoArgs,
	}
	command.AddCommand(showPublishingCommand(app), setPublishingCommand(app), removePublishingCommand(app))
	return command
}

func showPublishingCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show every setting, gitd.toml's included",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			answer, err := app.client().GetSettings(cmd.Context())
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				for _, settings := range answer.GetSettings() {
					settingsLines(out, settings)
				}
			})
			return nil
		},
	}
}

func setPublishingCommand(app *app) *cobra.Command {
	var preset, branchPrefix, open string
	var protected, kept, labels []string
	var maxFiles int32
	var draft, reviewRequester bool
	command := &cobra.Command{
		Use:   "set PREFIX",
		Short: "Set the rules of the repositories under PREFIX (\"\" for every one)",
		Example: "  gitd publishing set https://github.com/acme --preset recommended --branch-prefix agents/ \\\n" +
			"    --pull-requests always --draft --label agent --review-requester",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chosen, known := presetsByName[preset]
			if !known {
				return fmt.Errorf("no preset %q; there is \"recommended\"", preset)
			}
			settings := &gitdv1.PublishSettings{
				UrlPrefix: args[0], Preset: chosen, ProtectedPaths: protected, KeptPaths: kept, BranchPrefix: branchPrefix,
			}
			if cmd.Flags().Changed("max-files") {
				settings.MaxChangedFiles = &maxFiles
			}
			if open != "" {
				opening, known := openingsByName[open]
				if !known {
					return fmt.Errorf("--pull-requests is \"when-asked\" or \"always\", not %q", open)
				}
				settings.PullRequests = &gitdv1.PullRequestSettings{
					Open: opening, Draft: draft, Labels: labels, RequestReviewFromRequester: reviewRequester,
				}
			}
			answer, err := app.client().SetSettings(cmd.Context(), settings)
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { settingsLines(out, answer.GetSettings()) })
			return nil
		},
	}
	flags := command.Flags()
	flags.StringVar(&preset, "preset", "", "a ready set of rules: recommended")
	flags.StringArrayVar(&protected, "protect", nil, "a path a publish may not change, as a glob, e.g. 'deploy/**' (repeatable)")
	flags.StringArrayVar(&kept, "keep", nil, "a path a publish may not delete, as a glob (repeatable)")
	flags.Int32Var(&maxFiles, "max-files", 0, "at most this many files changed")
	flags.StringVar(&branchPrefix, "branch-prefix", "", "what every published branch must start with, e.g. agents/")
	flags.StringVar(&open, "pull-requests", "", "open pull requests: when-asked or always")
	flags.BoolVar(&draft, "draft", false, "open them as drafts")
	flags.StringArrayVar(&labels, "label", nil, "a label to put on them (repeatable)")
	flags.BoolVar(&reviewRequester, "review-requester", false, "ask whoever asked for the work to review")
	return command
}

func removePublishingCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "remove PREFIX",
		Short: "Forget the rules of a prefix set through the API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.client().RemoveSettings(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(app.stdout, "removed the publishing settings of %s\n", shownPrefix(args[0]))
			return nil
		},
	}
}

// settingsLines are a prefix's settings as people read them.
func settingsLines(out io.Writer, settings *gitdv1.PublishSettings) {
	title := shownPrefix(settings.GetUrlPrefix())
	if settings.GetFromFile() {
		title += "  (gitd.toml)"
	}
	fmt.Fprintln(out, title)
	if settings.GetPreset() != gitdv1.Preset_PRESET_UNSPECIFIED {
		fmt.Fprintf(out, "  preset: %s\n", strings.ToLower(strings.TrimPrefix(settings.GetPreset().String(), "PRESET_")))
	}
	if paths := settings.GetProtectedPaths(); len(paths) > 0 {
		fmt.Fprintf(out, "  protected: %s\n", strings.Join(paths, ", "))
	}
	if paths := settings.GetKeptPaths(); len(paths) > 0 {
		fmt.Fprintf(out, "  kept: %s\n", strings.Join(paths, ", "))
	}
	if settings.MaxChangedFiles != nil {
		fmt.Fprintf(out, "  at most %d files\n", settings.GetMaxChangedFiles())
	}
	if prefix := settings.GetBranchPrefix(); prefix != "" {
		fmt.Fprintf(out, "  branches: %s…\n", prefix)
	}
	if pullRequests := settings.GetPullRequests(); pullRequests != nil {
		open := strings.ToLower(strings.TrimPrefix(pullRequests.GetOpen().String(), "OPEN_"))
		line := "  pull requests: " + strings.ReplaceAll(open, "_", " ")
		if pullRequests.GetDraft() {
			line += ", drafts"
		}
		if len(pullRequests.GetLabels()) > 0 {
			line += ", labelled " + strings.Join(pullRequests.GetLabels(), ", ")
		}
		if pullRequests.GetRequestReviewFromRequester() {
			line += ", the requester asked to review"
		}
		fmt.Fprintln(out, line)
	}
}

func shownPrefix(prefix string) string {
	if prefix == "" {
		return "every repository"
	}
	return prefix
}
