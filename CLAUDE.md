# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

strata is a cross-platform dotfiles manager (macOS/Linux/Windows). A dotfiles repo mirrors
`$HOME` with real filenames; per-machine differences are **layers** (whole-file override), not
templates. It tracks the SHA-256 of everything it wrote so it can distinguish "the repo changed"
from "you edited the file in `$HOME`" from "both" — and refuses to clobber local edits.

Read `README.md` for the user-facing model (layer stacking, the seven file statuses, `dots.toml`
/ `machine.toml` schemas). `ROADMAP.md` records design decisions, including an explicit
**"not doing"** list — templates, symlink mode, partial-file merging, non-`$HOME` targets,
wrapping git, built-in secrets encryption. Don't propose those without acknowledging the
existing decision.

## Commands

No Makefile, no task runner — plain Go, single module (`module strata`, Go 1.26).

```sh
go build -o strata .                  # build
go test ./...                         # all tests (each runs in its own t.TempDir())
go vet ./... && gofmt -l .            # lint/format gate (gofmt must print nothing)

go test ./internal/engine -run TestPlanStatuses -v   # single test
go test . -run TestEndToEnd -v                       # the CLI end-to-end test
go test ./internal/tui -run TestDemoRender -v        # prints ANSI-stripped TUI frames

GOOS=linux go build   # cross-compile; also windows, darwin
sh install.sh         # build + install to ~/.local/bin (STRATA_BIN_DIR overrides)
```

CI (`.github/workflows/ci.yml`) runs `gofmt -l` (Linux only), `go vet`, `go build`, `go test` on
ubuntu/macos/windows for every push to `main` and every PR. Run the same gate locally first —
the Windows leg is the one that catches path-separator mistakes.

Releases are tag-driven: pushing a `v*` tag runs `release.yml` → GoReleaser (darwin/linux/windows
× amd64/arm64). Tags must be **un-padded** CalVer — `v2026.8.0`, not `v2026.08.0`; GoReleaser
enforces semver and rejects a zero-padded month.

## Architecture

```
main.go, cmd_*.go     cobra CLI, one file per command (cmd_tui.go = bare-strata launcher)
internal/config/      dots.toml + machine.toml parsing, var merging
internal/layers/      OS detection + layer resolution (rel path → winning source file)
internal/subst/       {{var}} substitution, fail-loud on undefined
internal/perms/       permission globs (doublestar; longest pattern wins)
internal/state/       last-applied-hash store (state.json)
internal/engine/      Plan (status classification) → Apply → RunHooks
internal/fsutil/      SHA-256 + atomic write (temp file + rename)
internal/tui/         read-only Bubble Tea TUI: snapshot.go (data) / model.go / view.go
                      theme.go holds every color and lipgloss style — no literals elsewhere
```

**The pipeline.** Every command funnels through `loadContext()` in [main.go](main.go), the only
place that touches disk config: resolve paths → `config.LoadMachineConfig` →
`config.LoadRepoConfig` → `state.Load` → `config.Merge`. Then `app.plan()` calls
`engine.Plan`, which is the single source of truth for "what would apply do":

```
layers.Order(roles, goos, osRelease)  →  layers.Resolve(repoDir, order)   # rel → winning source
  →  read file  →  subst.Apply (only if rel is in cfg.Substitute)  →  perms.ModeFor
  →  compare desired / current / last-applied-hash  →  engine.FileStatus
```

`engine.Apply` then writes only `Create`/`Update` items and deletes `Removed` ones.

### Invariants to preserve

- **Platform is a parameter, never ambient.** `engine.Plan` and `tui.Build` take `goos` and
  `osRelease` as arguments; only `main.go` supplies `runtime.GOOS` and
  `layers.ReadOSRelease()`. This is what lets tests exercise mac/arch/windows behavior on any
  host, and the TUI resolve all four OS columns at once. Never call `runtime.GOOS` inside
  `internal/` resolution code.
- **All-or-nothing apply.** `engine.Apply` collects every blocked item (`drifted` / `conflict` /
  `unmanaged`, plus a `removed` file edited since the last apply) in a first pass and returns an
  error *before* writing anything. A half-applied state must stay impossible.
- **Writes go through `fsutil.WriteFileAtomic`.** Temp file + `Chmod` + `rename`. Applies to
  `state.json` too.
- **Rel paths are forward-slash everywhere** — map keys, state.json, `dots.toml` patterns.
  Convert with `filepath.FromSlash` only at the moment you touch disk (`filepath.Join(home,
  filepath.FromSlash(rel))`). Windows support depends on this discipline.
- **Substitution is opt-in and fail-loud.** Only files listed in `dots.toml`'s `substitute` get
  `{{var}}` replaced, because dotfiles are full of `${VAR}` and other tools' `{{ }}`. An
  undefined var aborts the whole apply.
- **`Clean` files get adopted into state.** `Apply` records the hash for `Clean` items as well
  as written ones — that's how a machine with pre-existing identical dotfiles stops reporting
  `unmanaged`.
- **The TUI is strictly read-only** by design. `internal/tui` may only read. Adding write
  actions is a roadmap "someday, opt-in" item, not a default.

### Gotcha: `FileStatus`

`engine.FileStatus` is an iota enum whose `String()` is a **positional array literal**. Adding
or reordering a status silently misnames every later one. Update the const block, the array, and
the status table in `README.md` together.

## Conventions

- **Commands write to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`**, never `fmt.Println`. The
  e2e test in [e2e_test.go](e2e_test.go) builds the real `newRootCmd()`, redirects output into a
  buffer, and asserts on it — bare stdout writes are invisible to it.
- Each command lives in `cmd_<name>.go` exposing `newXxxCmd() *cobra.Command`, registered in
  `newRootCmd()`. Cobra `Long`/`Example` text is substantive here (it's the real help); keep it
  in sync when behavior changes.
- Tests build fixture repos with a local `mk(rel, content)` helper into `t.TempDir()` and set
  `STRATA_HOME` / `STRATA_CONFIG` / `STRATA_STATE` via `t.Setenv`. Those env vars (plus
  `STRATA_BIN`, which `uninstall` deletes) are the sandbox seam — use them rather than mocking
  the filesystem, and never let a test touch the real `$HOME`.
- Version lives in `var version` in [main.go](main.go) — CalVer `YYYY.M.PATCH`. It must stay a
  `var`, not a `const`: release builds overwrite it via GoReleaser's
  `-X main.version={{.Version}}` ldflag, and `-X` silently does nothing to a `const`.
  `install.sh` stamps it too, from `git describe --tags --dirty --always`. The in-source value
  is only the fallback for a bare `go build` or a tarball with no git history.
- Package doc comments carry the "why" for each `internal/` package. Match that density.

## Repo notes

- `.gitignore` excludes the built `/strata` binary, `dist/`, `.claude/`, and `/docs/`. Every
  ignored path is deliberate — treat the list as load-bearing and don't prune entries that look
  unused just because the directory isn't in a fresh clone. That's precisely why they're ignored.
