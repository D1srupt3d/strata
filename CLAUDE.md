# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

strata is a cross-platform dotfiles manager (macOS/Linux/Windows). A dotfiles repo mirrors
`$HOME` with real filenames; per-machine differences are **layers** (whole-file override), not
templates. It tracks the SHA-256 of everything it wrote so it can distinguish "the repo changed"
from "you edited the file in `$HOME`" from "both" — and refuses to clobber local edits.

Read `README.md` for the user-facing model (layer stacking, the eight file statuses, `dots.toml`
/ `machine.toml` schemas). `ROADMAP.md` records design decisions, including an explicit
**"not doing"** list — templates, symlink mode, partial-file merging, non-`$HOME` targets,
wrapping git, built-in secrets encryption. Don't propose those without acknowledging the
existing decision.

## Commands

No Makefile, no task runner — plain Go, single module (`module strata`, Go 1.26).

```sh
go build -o strata .                  # build
go test ./...                         # all tests (each runs in its own t.TempDir())
go test -race ./...                   # race detector (CI runs it on Linux)
go vet ./... && gofmt -l .            # lint/format gate (gofmt must print nothing)
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...   # linter (CI, pinned version)
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...    # vulnerability scan (CI, pinned version)
go test -fuzz=FuzzVerifySSHSig ./internal/release        # fuzz the signature parser (also: FuzzApply in internal/subst)

go test ./internal/engine -run TestPlanStatuses -v   # single test
go test . -run TestEndToEnd -v                       # the CLI end-to-end test
go test ./internal/tui -run TestDemoRender -v        # prints ANSI-stripped TUI frames

GOOS=linux go build   # cross-compile; also windows, darwin
sh install.sh         # build + install to ~/.local/bin (STRATA_BIN_DIR overrides)
```

CI (`.github/workflows/ci.yml`) runs `gofmt -l` and `go test -race` (Linux only), and `go vet`,
`go build`, `go test`, staticcheck and govulncheck on ubuntu/macos/windows for every push to `main`
and every PR. The linters run on every OS because they only analyze code built for the OS they run
on (`lock_windows.go` is invisible to a Linux run); to check another OS locally, `go install` them
and run with `GOOS=windows` (a `go run` with `GOOS` set builds a binary this machine can't run). The two linters run via `go run pkg@vX.Y.Z`, pinned to exact versions — bump them
deliberately. govulncheck scans the standard library of the Go that runs it, and CI (and the
release build) use the `go` version in `go.mod` — so to match CI locally run it as
`GOTOOLCHAIN=go<that version> go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`. A newer
local Go can report clean while CI fails. When it flags the standard library, bump the `go` line
in `go.mod` to the fixed patch release. Run the same
gate locally first — the Windows leg is the one that catches path-separator mistakes. Workflow
actions are pinned to full commit SHAs with a `# vX.Y.Z` comment (Renovate bumps both); keep it
that way when adding steps.

Releases are tag-driven: pushing a `v*` tag runs `release.yml` → GoReleaser (darwin/linux/windows
× amd64/arm64), then attaches signed build provenance to every archive and `checksums.txt`
(verify with `gh attestation verify <file> --repo D1srupt3d/strata`). GoReleaser's `signs:` step
signs `checksums.txt` with the release SSH key (`ssh-keygen -Y sign -n strata-release`) →
`checksums.txt.sig`; the private key is the `STRATA_RELEASE_KEY` secret in the GitHub environment
`release` (deployment rule: `v*` tags only). release.yml fails if the secret is missing and verifies
the published signature afterwards — **never publish an unsigned release**; `strata upgrade` and
`get.sh` refuse them. Check GoReleaser config locally with
`go run github.com/goreleaser/goreleaser/v2@latest check`. Tags must be **un-padded**
CalVer — `v2026.8.0`, not `v2026.08.0`; GoReleaser enforces semver and rejects a zero-padded month.

## Architecture

```
main.go, cmd_*.go     cobra CLI, one file per command (cmd_tui.go = bare-strata launcher)
internal/config/      dots.toml + machine.toml parsing, var merging
internal/layers/      OS detection + layer resolution (rel path → winning source file)
internal/subst/       {{var}} substitution, fail-loud on undefined
internal/perms/       permission globs (doublestar; longest pattern wins, equal-length disagreement errors)
internal/state/       state.json: last-applied hashes, pending-hook queue, format version, file lock
internal/engine/      Plan (status classification) → Apply → RunHooks
internal/doctor/      `strata doctor`: every setup check run on its own, all problems listed;
                      read-only, and reuses config/layers/perms/subst/state so it reports what apply would
internal/fsutil/      SHA-256 + atomic write (temp file + fsync + rename)
internal/release/     `strata upgrade`: find, verify (SSH signature, stdlib only), and install signed
                      releases; releasetest/ fakes a signed GitHub release for tests (test-only)
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

`engine.Apply` then writes `Create`/`Update` items, chmods `Chmod` ones, and deletes `Removed`
ones. Every command that applies (`apply`, `edit`, `rm`, `init`, `sync`) goes through `runApply` in
[cmd_apply.go](cmd_apply.go) — never by invoking another command's `RunE`. Var provenance
(`Config.VarFrom`) is decided in `config.Merge`; the TUI displays it and must not re-derive it.

### Invariants to preserve

- **Platform is a parameter, never ambient.** `engine.Plan` and `tui.Build` take `goos` and
  `osRelease` as arguments; only `main.go` supplies `runtime.GOOS` and
  `layers.ReadOSRelease()`. This is what lets tests exercise mac/arch/windows behavior on any
  host, and the TUI resolve all four OS columns at once. Never call `runtime.GOOS` inside
  `internal/` resolution code.
- **All-or-nothing apply.** `engine.Apply` collects every blocked item in a first pass and returns
  an error *before* writing anything. `Item.Blocked` is the single definition of "blocked"
  (drifted / conflict / unmanaged, an update or chmod that would replace a `$HOME` symlink, a
  removed file edited since the last apply) — `--dry-run` and `rm`'s up-front refusal use it too
  (`engine.BlockedList` formats the list). A write that fails for an I/O reason partway still
  returns the partial `ApplyResult` (`Changed()`); `runApply` saves state and queues hooks for
  what was written before returning the error, or those hooks would never run.
- **Writes go through `fsutil.WriteFileAtomic`.** Temp file + `Chmod` + fsync + `rename`. Applies
  to `state.json` too. Missing parent folders are created 0700 when the file gives group/others
  nothing, else 0755; existing folders are never changed.
- **Role layers must exist.** `engine.CheckLayers` (first thing in `Plan`, and in `tui.Build`
  before any walk) rejects a role layer that isn't a folder in the repo, or isn't a single folder
  name — a skipped typo made its files read `removed`. The one exception is a repo folder that
  doesn't exist at all, which reads as an empty repo (README "Order matters"). `machine.toml`'s
  `repo` must be absolute after `~` expansion, or layers would resolve against the cwd.
- **`add` records state only for the winning layer.** When `--layer` isn't the layer this machine
  gets the file from (`winningLayer`), the `$HOME` copy is left unrecorded — recording it made
  the next apply overwrite it, or delete it as `removed`.
- **state.json paths are never trusted for deletion.** `Plan` refuses an entry that isn't
  `filepath.IsLocal`; a hand-edited `../x` must not reach `os.Remove`.
- **Hooks are queued before they run.** `runApply` saves the pending-hook queue to state *before*
  running hooks and clears each only on success, so a failed or interrupted hook reruns on the
  next apply. Hooks run with `$HOME` as the working directory.
- **State mutations lock, then re-read.** Anything that saves state takes `state.Lock` and loads
  state again under it; saving a copy loaded before the lock would drop another process's
  updates.
- **Modes are enforced only when explicit.** `Chmod` fires only for an explicit `[permissions]`
  rule or a missing exec bit, and never for `goos == "windows"`; the 644 default must never loosen
  a mode the user tightened by hand.
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
or reordering a status silently misnames every later one. Append new statuses at the end, and
update the const block, the array, the TUI's `statusGlyph`/`driftLabel`, `status --help`, and the
status table in `README.md` together.

## Conventions

- **Commands write to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`**, never `fmt.Println`. The
  e2e test in [e2e_test.go](e2e_test.go) builds the real `newRootCmd()`, redirects output into a
  buffer, and asserts on it — bare stdout writes are invisible to it.
- Each command lives in `cmd_<name>.go` exposing `newXxxCmd() *cobra.Command`, registered in
  `newRootCmd()`. Cobra `Long`/`Example` text is substantive here (it's the real help); keep it
  in sync when behavior changes.
- CLI tests use [helpers_test.go](helpers_test.go): `sandbox(t, layers...)` builds an isolated
  repo + home in `t.TempDir()` and sets `STRATA_HOME` / `STRATA_CONFIG` / `STRATA_STATE`;
  `writeFile`/`readFile` fail the test on setup errors (never ignore them); `runIn` feeds stdin
  to prompts; `isolateGit` must wrap any test that runs git — it blanks the developer's global
  config, whose commit signing would otherwise pop a password-manager prompt. Those env vars
  (plus `STRATA_BIN`, which `uninstall` deletes) are the sandbox seam — use them rather than
  mocking the filesystem, and never let a test touch the real `$HOME`.
- A command reports "needs attention" with `exitCode(n)` (exit status, no `error:` line — see
  `status`); real failures return a normal error.
- Version lives in `var version` in [main.go](main.go) — CalVer `YYYY.M.PATCH`. It must stay a
  `var`, not a `const`: release builds overwrite it via GoReleaser's
  `-X main.version={{.Version}}` ldflag, and `-X` silently does nothing to a `const`.
  `install.sh` stamps it too, from `git describe --tags --dirty --always`. The in-source value
  is only the fallback for a bare `go build` or a tarball with no git history.
- `var channel` in main.go works like `version`: GoReleaser stamps `-X main.channel=release`;
  every other build is `source`. Only `release` binaries `strata upgrade` themselves — source and
  Homebrew installs are refused by design.
- The release **public** key lives in two places: `internal/release/release_key.pub` (embedded)
  and `get.sh` (which can't read the repo when piped from curl). `TestGetShKeyMatchesEmbeddedReleaseKey`
  keeps them identical — change both together. Signature test fixtures come from the real
  `ssh-keygen` via `internal/release/testdata/gen.sh` (throwaway key; never commit a private key).
- Package doc comments carry the "why" for each `internal/` package. Match that density.

## Repo notes

- `.gitignore` excludes the built `/strata` binary, `dist/`, `.claude/`, and `/docs/`. Every
  ignored path is deliberate — treat the list as load-bearing and don't prune entries that look
  unused just because the directory isn't in a fresh clone. That's precisely why they're ignored.
