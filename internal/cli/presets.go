package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func presetCommand(app *app) *cobra.Command {
	command := &cobra.Command{
		Use:   "preset",
		Short: "List, set and remove presets, the named sets of rules settings take",
		Long: "A preset names protected and kept paths and a file limit, for settings to take by name\n" +
			"(`gitd publishing set PREFIX --preset NAME`). gitd has its own, recommended; a preset\n" +
			"of that name replaces it, and removing that one brings gitd's back.",
		Args: cobra.NoArgs,
	}
	command.AddCommand(listPresetsCommand(app), setPresetCommand(app), removePresetCommand(app))
	return command
}

func listPresetsCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every preset as it applies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			answer, err := app.client().ListPresets(cmd.Context())
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) {
				for _, preset := range answer.GetPresets() {
					presetLines(out, preset)
				}
			})
			return nil
		},
	}
}

func setPresetCommand(app *app) *cobra.Command {
	var protected, kept []string
	var maxFiles int32
	command := &cobra.Command{
		Use:     "set NAME",
		Short:   "Keep a preset, replacing one of the same name",
		Example: "  gitd publishing preset set strict --protect '.github/**' --protect 'deploy/**' --keep 'migrations/**' --max-files 50",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			preset := &gitdv1.Preset{Name: args[0], ProtectedPaths: protected, KeptPaths: kept}
			if cmd.Flags().Changed("max-files") {
				preset.MaxChangedFiles = &maxFiles
			}
			answer, err := app.client().SetPreset(cmd.Context(), preset)
			if err != nil {
				return err
			}
			app.print(answer, func(out io.Writer) { presetLines(out, answer.GetPreset()) })
			return nil
		},
	}
	command.Flags().StringArrayVar(&protected, "protect", nil, "a path a publish may not change, as a glob (repeatable)")
	command.Flags().StringArrayVar(&kept, "keep", nil, "a path a publish may not delete, as a glob (repeatable)")
	command.Flags().Int32Var(&maxFiles, "max-files", 0, "at most this many files changed")
	return command
}

func removePresetCommand(app *app) *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Forget a preset set through the API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.client().RemovePreset(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(app.stdout, "removed the preset %s\n", args[0])
			return nil
		},
	}
}

// presetLines are a preset as people read it.
func presetLines(out io.Writer, preset *gitdv1.Preset) {
	origin := strings.ToLower(strings.TrimPrefix(preset.GetOrigin().String(), "ORIGIN_"))
	fmt.Fprintf(out, "%s  (%s)\n", preset.GetName(), strings.ReplaceAll(origin, "_", "-"))
	if paths := preset.GetProtectedPaths(); len(paths) > 0 {
		fmt.Fprintf(out, "  protected: %s\n", strings.Join(paths, ", "))
	}
	if paths := preset.GetKeptPaths(); len(paths) > 0 {
		fmt.Fprintf(out, "  kept: %s\n", strings.Join(paths, ", "))
	}
	if preset.MaxChangedFiles != nil {
		fmt.Fprintf(out, "  at most %d files\n", preset.GetMaxChangedFiles())
	}
}
