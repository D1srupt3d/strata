package main

import (
	"slices"
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
	return managedRels(toComplete, nil), cobra.ShellCompDirectiveNoFileComp
}

// completeManagedFiles is completeManaged for commands that take several
// files (diff): every Tab offers the managed files not already on the line.
func completeManagedFiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return managedRels(toComplete, args), cobra.ShellCompDirectiveNoFileComp
}

// managedRels lists the managed files starting with prefix, minus skip.
func managedRels(prefix string, skip []string) []string {
	app, err := loadContext()
	if err != nil {
		return nil
	}
	items, err := app.plan()
	if err != nil {
		return nil
	}
	var rels []string
	for _, it := range items {
		// A removed file has no source left to edit or delete.
		if it.Source != "" && strings.HasPrefix(it.Rel, prefix) && !slices.Contains(skip, it.Rel) {
			rels = append(rels, it.Rel)
		}
	}
	return rels
}
