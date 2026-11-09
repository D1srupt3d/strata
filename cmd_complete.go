package main

import (
	"strings"

	"github.com/spf13/cobra"
)

// completeManaged is Tab completion for commands that take one managed file
// (edit, rm): the files some layer provides, filtered by what's typed. It
// never falls back to the shell's own file names, which would offer files
// strata doesn't manage, and stays quiet when strata isn't set up - an
// error here would land in the middle of the user's command line.
func completeManaged(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	app, err := loadContext()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items, err := app.plan()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var rels []string
	for _, it := range items {
		// A removed file has no source left to edit or delete.
		if it.Source != "" && strings.HasPrefix(it.Rel, toComplete) {
			rels = append(rels, it.Rel)
		}
	}
	return rels, cobra.ShellCompDirectiveNoFileComp
}
