package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <file>",
		Short: "Open the winning layer's source in your editor, then offer to apply",
		Long: `Opens the source file in the layer that wins on this machine — you never
have to remember which layer that is. When the editor exits, shows the
diff and offers to apply immediately (edit-and-apply in one step).

The editor is $VISUAL, else $EDITOR, else vi, and may include arguments.
GUI editors must be told to wait until the file is closed ("code --wait"),
or strata shows the diff before you've made your edit.

To edit a non-winning layer's copy (e.g. base/.gitconfig while work/
overrides it), just open that file directly — it's a plain file.`,
		Example: `  strata edit .zshrc
  EDITOR="code --wait" strata edit .gitconfig`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			rel, err := relFromArg(args[0], app.Paths.Home)
			if err != nil {
				return err
			}
			items, err := app.plan()
			if err != nil {
				return err
			}
			source := ""
			for _, it := range items {
				if it.Rel == rel {
					source = it.Source
				}
			}
			if source == "" {
				return fmt.Errorf("%s is not managed (try: strata add %s)", rel, rel)
			}
			ed := editorCommand(source)
			ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := ed.Run(); err != nil {
				return fmt.Errorf("editor: %w", err)
			}
			// Show what changed and offer to apply.
			if err := writeDiff(app, cmd.OutOrStdout()); err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), "apply now? [y/N] ")
			line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			if strings.TrimSpace(strings.ToLower(line)) == "y" {
				return runApply(app, cmd.OutOrStdout(), applyOpts{})
			}
			return nil
		},
	}
}

// editorCommand runs the user's editor on file: $VISUAL, then $EDITOR, then
// vi — git's order. The value may carry arguments ("code --wait"), so on
// Unix it goes through sh exactly as git does; Windows splits on spaces.
func editorCommand(file string) *exec.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	if runtime.GOOS == "windows" {
		f := strings.Fields(editor)
		return exec.Command(f[0], append(f[1:], file)...)
	}
	return exec.Command("sh", "-c", editor+` "$@"`, editor, file)
}
