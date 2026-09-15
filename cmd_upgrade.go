package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/release"
)

// upgradeKeys are the release-signing keys upgrade trusts. A var so tests
// can swap in a throwaway key; releases use the embedded release key.
var upgradeKeys = release.TrustedKeys

func newUpgradeCmd() *cobra.Command {
	var check, force bool
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update"},
		Short:   "Replace this strata with the latest signed release",
		Long: `Downloads the latest strata release from GitHub, verifies it, and replaces
this binary with it. strata never touches the network unless you run this.

Everything is checked before anything is replaced: the signature on the
release's checksums (made by the strata release key), that the release is
newer than this build, the archive's SHA-256, and that the new binary runs
and reports the expected version. If any check fails, the installed strata
is left exactly as it was.

Only release builds replace themselves:
  - Homebrew installs: use 'brew upgrade strata'.
  - Built from source (install.sh, go build): update it the way you built it
    — 'git pull && sh install.sh' — or switch to release builds with get.sh.`,
		Example: `  strata upgrade           install the latest release
  strata upgrade --check   is there a newer release? (exit 1 if so)
  strata upgrade --force   reinstall the current release`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			p, err := resolvePaths()
			if err != nil {
				return err
			}
			target := p.Bin
			if target == "" {
				return errors.New("can't locate the running strata binary")
			}
			if resolved, err := filepath.EvalSymlinks(target); err == nil {
				target = resolved // replace the real file, not a symlink to it
			}
			if isHomebrewPath(target) {
				return fmt.Errorf("%s is managed by Homebrew — run 'brew upgrade strata'", target)
			}
			release.CleanupOld(target)
			opts := release.Options{
				APIURL:  os.Getenv("STRATA_RELEASE_API"),
				GOOS:    runtime.GOOS,
				GOARCH:  runtime.GOARCH,
				Target:  target,
				Trusted: upgradeKeys,
				Force:   force,
			}

			if channel != "release" {
				if !check {
					return fmt.Errorf("this strata (%s) was built from source, so it's yours to update: 'git pull && sh install.sh' in your checkout — or switch to release builds with get.sh", version)
				}
				latest, _, err := release.Latest(ctx, opts)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "latest release: %s (this strata, %s, is a source build — update it with 'git pull && sh install.sh')\n", latest, version)
				return nil
			}

			current, err := release.ParseVersion(version)
			if err != nil {
				return fmt.Errorf("this release build has an unreadable version: %w", err)
			}
			opts.Current = current
			if check {
				latest, newer, err := release.Latest(ctx, opts)
				if err != nil {
					return err
				}
				if newer {
					fmt.Fprintf(out, "update available: %s → %s (run 'strata upgrade')\n", current, latest)
					return exitCode(1)
				}
				fmt.Fprintf(out, "strata %s is up to date\n", current)
				return nil
			}

			res, err := release.Upgrade(ctx, opts)
			if err != nil {
				return err
			}
			if !res.Upgraded {
				fmt.Fprintf(out, "strata %s is up to date\n", res.From)
				return nil
			}
			fmt.Fprintf(out, "upgraded strata %s → %s\n", res.From, res.To)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only report whether a newer release exists (exit 1 if so)")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if this is already the latest release")
	return cmd
}

// isHomebrewPath reports whether a binary lives in a Homebrew prefix.
// Homebrew owns those files and its own bookkeeping, so strata defers to it.
func isHomebrewPath(p string) bool {
	p = filepath.ToSlash(p)
	for _, marker := range []string{"/Cellar/", "/opt/homebrew/", "/home/linuxbrew/.linuxbrew/", "/usr/local/Homebrew/"} {
		if strings.Contains(p, marker) {
			return true
		}
	}
	return false
}
