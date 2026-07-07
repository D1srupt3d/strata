# strata roadmap

Versioning is CalVer (`YYYY.0M.PATCH`). Dates are targets, not promises —
this is a personal tool; items ship when they're ready and needed.

## Shipped — 2026.07.0

- Core engine: layered resolution (base → OS → distro → role), copy-on-apply,
  three-way drift detection, atomic writes, all-or-nothing apply
- Opt-in `{{var}}` substitution, permission globs, per-file post-apply hooks
- CLI: init / apply / diff / status / edit / add / sync, with full help text
- Read-only TUI (bare `strata`): Layers / Files / Vars & Rules + drilldown
- Installer with PATH setup; cross-compiles darwin/linux/windows
- Proven in production: real chezmoi migration, byte-verified, chezmoi retired

## Next — 2026.07.1: file removal tracking

The biggest daily-use gap: deleting a file from the repo currently orphans
it in `$HOME` forever.

- Track managed files in state; when a file disappears from every layer,
  `status` shows it as `removed` and `apply` deletes it from `$HOME`
  (with the same refuse-if-locally-edited safety as overwrites)
- `strata rm <file>` convenience: delete from the winning layer + apply

## Soon

- **CI + releases**: GitHub Actions (test + vet + cross-build on push),
  tagged releases with prebuilt binaries so a new machine is
  `curl … && strata init <url>` — no Go toolchain needed
- **Second-machine validation**: real `strata init` bootstrap on the
  personal Mac; fix whatever that run surfaces (it always surfaces something)
- **Linux run-through**: the arch layer logic is tested but has never
  executed on real Linux

## Later (as need arises)

- **Conflict assist**: three-way merge option for `conflict` files instead
  of pick-a-side only
- **Directory permissions**: `[permissions]` globs for dirs (e.g. `.gnupg` 700)
- **Hook improvements**: glob keys (`".config/nvim/**"`), a `run_once`
  equivalent for machine bootstrap steps
- **TUI actions**: opt-in write mode — `a` to apply a file, `d`iff already
  exists; keeps read-only as the default
- **Secrets stance**: likely stays "keep secrets out of the repo" +
  documentation for 1Password/age integration patterns rather than
  built-in encryption

## Non-goals

- Templating language (layers + vars cover the real cases)
- Symlink mode, partial-file merging
- Managing files outside `$HOME`
- Wrapping git beyond `sync`
