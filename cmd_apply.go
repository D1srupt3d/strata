package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"strata/internal/engine"
	"strata/internal/state"
)

// applyOpts are apply's flags. edit, init and sync call runApply with the
// zero value, so every path into apply behaves identically - and none of
// them deletes a file. rm sets prune to just the file it removed.
type applyOpts struct {
	dryRun, force bool
	prune         engine.Prune
}

func newApplyCmd() *cobra.Command {
	var opts applyOpts
	var prune bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Build files from layers + vars and copy changes into $HOME, then run hooks",
		Long: `Builds every managed file (stack layers → substitute {{vars}} → resolve
permissions) and copies the ones that changed into $HOME, then runs hooks
for the files that were written.

Safety rules:
  - If ANY file is drifted/conflicted/unmanaged, apply writes NOTHING and
    lists them - keep your version with 'strata add <file>', or take the
    repo's with --force. All-or-nothing: one blocked file stops them all.
  - Writes are atomic (temp file + rename); a crash never leaves a
    half-written dotfile. If a write fails partway (disk full, a folder
    you can't write to), the files already written are recorded and their
    hooks queued, so the next apply picks up where this one stopped.
  - An undefined {{var}} in a substituted file aborts before anything is
    written.
  - A file strata wrote that's no longer in the repo (status 'removed') is
    kept in $HOME unless you pass --prune: strata can't tell a file you
    deleted from one that's only missing because the repo is on another
    git branch. --prune still refuses a file you edited since the last
    apply; add --force to delete it anyway.`,
		Example: `  strata apply --dry-run    preview without writing
  strata apply              write changes, run hooks
  strata apply --force      also overwrite drifted/conflicting files
  strata apply --prune      also delete files that left the repo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			if prune {
				opts.prune = engine.PruneAll
			}
			return runApply(app, cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "n", false, "show what would change without writing")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite drifted/conflicting/unmanaged files")
	cmd.Flags().BoolVar(&prune, "prune", false, "delete files that are no longer in the repo from $HOME")
	return cmd
}

// printPlan is --dry-run: what apply would do, file by file, including the
// all-or-nothing rule - one blocked file means apply writes nothing at all.
func printPlan(out io.Writer, items []engine.Item, st state.State, hooks map[string]string, force bool, prune engine.Prune) {
	lines, blocked := 0, 0
	var hooked []string
	for _, it := range items {
		var verb string
		switch {
		case it.Status == engine.Clean:
			continue
		case it.Blocked(st, prune) && !force:
			reason := "keep yours with 'strata add', or take the repo's with --force"
			if it.Symlink {
				reason = "it's a symlink strata won't replace; --force swaps in a regular file"
			}
			if it.Status == engine.Removed {
				reason = "edited since the last apply; leave out --prune to keep it, or add --force to delete it"
			}
			fmt.Fprintf(out, "%-12s %s (%s: %s)\n", "blocked", it.Rel, it.Status, reason)
			lines++
			blocked++
			continue
		case it.Status == engine.Removed && !prune.Selects(it.Rel):
			if it.Current == nil {
				continue // already gone from $HOME: only a stale state entry to drop
			}
			fmt.Fprintf(out, "%-12s %s (removed: not in the repo; --prune deletes it)\n", "would keep", it.Rel)
			lines++
			continue
		case it.Status == engine.Removed:
			verb = "would remove"
		case it.Status == engine.Chmod:
			verb = "would chmod"
		default: // create, update, or a --force overwrite
			verb = "would write"
			if _, ok := hooks[it.Rel]; ok {
				hooked = append(hooked, it.Rel)
			}
		}
		fmt.Fprintf(out, "%-12s %s (%s)\n", verb, it.Rel, it.Status)
		lines++
	}
	queue := state.State{PendingHooks: append([]string(nil), st.PendingHooks...)}
	queue.QueueHooks(hooked)
	for _, rel := range queue.PendingHooks {
		if _, ok := hooks[rel]; ok {
			fmt.Fprintf(out, "%-12s %s\n", "would hook", rel)
			lines++
		}
	}
	switch {
	case blocked > 0:
		fmt.Fprintf(out, "\napply would refuse: nothing is written until the %d blocked file(s) are resolved\n", blocked)
	case lines == 0:
		fmt.Fprintln(out, "nothing to do")
	}
}

// runApply plans, writes, saves state, then runs hooks - every hook still
// pending, which includes ones that failed on an earlier apply.
func runApply(app *appContext, out io.Writer, opts applyOpts) error {
	if !opts.dryRun {
		unlock, err := state.Lock(app.Paths.State)
		if err != nil {
			return err
		}
		defer unlock()
		// Re-read under the lock: another strata may have saved since this
		// context was loaded, and saving a stale copy would drop its updates.
		if app.State, err = state.Load(app.Paths.State); err != nil {
			return err
		}
	}
	items, err := app.plan()
	if err != nil {
		return err
	}
	if opts.dryRun {
		printPlan(out, items, app.State, app.Cfg.Hooks, opts.force, opts.prune)
		return nil
	}
	res, applyErr := engine.Apply(items, app.Paths.Home, &app.State, opts.force, opts.prune)
	if applyErr != nil && !res.Changed() {
		return applyErr // refused, or failed before touching $HOME
	}
	// Queue hooks and persist the queue BEFORE running any: a hook that
	// fails - or a crash mid-hook - stays pending and reruns next apply.
	// This also runs when a write failed partway: the files that did get
	// written must be recorded with their hooks queued, or the next apply
	// sees them clean and their hooks never run.
	var hooked []string
	for _, rel := range res.Written {
		if _, ok := app.Cfg.Hooks[rel]; ok {
			hooked = append(hooked, rel)
		}
	}
	app.State.QueueHooks(hooked)
	if err := app.State.Save(app.Paths.State); err != nil {
		return errors.Join(applyErr, err)
	}
	for _, rel := range res.Written {
		fmt.Fprintf(out, "wrote %s\n", rel)
	}
	for _, rel := range res.Chmodded {
		fmt.Fprintf(out, "chmod %s\n", rel)
	}
	for _, rel := range res.Deleted {
		fmt.Fprintf(out, "removed %s\n", rel)
	}
	for _, rel := range res.Kept {
		fmt.Fprintf(out, "kept %s (not in the repo anymore - 'strata apply --prune' deletes it)\n", rel)
	}
	if applyErr != nil {
		return fmt.Errorf("%w\n(what was written before the error is recorded and its hooks are queued - fix the problem, then run 'strata apply' again)", applyErr)
	}
	pending := app.State.PendingHooks
	if !res.Changed() && len(res.Kept) == 0 && len(pending) == 0 {
		fmt.Fprintln(out, "nothing to do")
	}
	if len(pending) == 0 {
		return nil
	}
	done, hookErr := engine.RunHooks(app.Cfg.Hooks, pending, app.Paths.Home, out)
	app.State.FinishHooks(done)
	if err := app.State.Save(app.Paths.State); err != nil {
		return errors.Join(hookErr, err)
	}
	if hookErr != nil {
		return fmt.Errorf("%w\n(the failed hook stays pending; the next 'strata apply' retries it)", hookErr)
	}
	return nil
}
