package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <file>",
		Short: "Open the winning layer's source in $EDITOR, then offer to apply",
		Args:  cobra.ExactArgs(1),
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
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			ed := exec.Command(editor, source)
			ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := ed.Run(); err != nil {
				return err
			}
			// Show what changed and offer to apply.
			diff := newDiffCmd()
			diff.SetOut(cmd.OutOrStdout())
			if err := diff.RunE(diff, nil); err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), "apply now? [y/N] ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if strings.TrimSpace(strings.ToLower(line)) == "y" {
				apply := newApplyCmd()
				apply.SetOut(cmd.OutOrStdout())
				return apply.RunE(apply, nil)
			}
			return nil
		},
	}
}
