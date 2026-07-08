# strata

**Layered dotfiles, sanely.**

strata is a cross-platform dotfiles manager (macOS, Linux, Windows) built around three ideas:

1. **Your repo mirrors your home directory.** Files keep their real names — `base/.zshrc`, not `dot_zshrc`. Grep works, tab-completion works, GitHub renders it like a home directory.
2. **Machine differences are layers, not templates.** A `work/` folder overrides a `base/` folder. No template language to learn for the common case.
3. **You can't lose local edits.** strata remembers what it wrote, so it always knows the difference between "the repo changed", "you edited the file", and "both" — and refuses to clobber your work.


```
strata edit .zshrc     # open the real source file, see the diff, apply — one step
strata diff            # what would change, in BOTH directions (repo→home and home→repo)
strata apply           # copy changes into $HOME, run hooks, never clobber local edits
```

And running **bare `strata`** opens a read-only [terminal UI](#the-tui-bare-strata) that shows the whole picture at a glance: which layer wins for every file on every OS, what state each file is in, and where every variable, hook, and permission comes from.

---

## Table of contents

- [Quick start](#quick-start)
- [Core concepts](#core-concepts)
- [The TUI (bare `strata`)](#the-tui-bare-strata)
- [Command reference](#command-reference)
- [Configuration reference](#configuration-reference)
- [Everyday workflows](#everyday-workflows)
- [How the safety model works](#how-the-safety-model-works)
- [Environment variables](#environment-variables)
- [Building and testing](#building-and-testing)
- [Security note](#security-note)
- [What strata deliberately doesn't do (yet)](#what-strata-deliberately-doesnt-do-yet)

---

## Quick start

### Install

```sh
sh install.sh
```

That builds the binary, copies it to `~/.local/bin/strata`, and — if that directory isn't on your `PATH` — appends one `export PATH=...` line to your shell rc (`.zshrc`/`.bashrc`/`.profile`, matched to your `$SHELL`). It's idempotent: re-run it any time to upgrade; it never adds the line twice. `STRATA_BIN_DIR=/somewhere sh install.sh` overrides the destination.

> **If your shell rc is itself managed by your dotfiles** (it will be, eventually — that's the point of strata), put the `export PATH="$HOME/.local/bin:$PATH"` line in your repo's `base/.zshrc` instead, so the installer finds it already present and leaves your rc alone.

Manual alternative: `go build -o strata .` and move the binary anywhere on your `PATH`. On Windows: `go build -o strata.exe .` and place it in a directory listed in your `Path` environment variable.

Cross-compiles with plain Go: `GOOS=linux go build`, `GOOS=windows go build`, etc.

### Starting from scratch

```sh
# 1. Make a dotfiles repo and put a file in the shared layer
mkdir -p ~/dotfiles/base
cp ~/.zshrc ~/dotfiles/base/.zshrc

# 2. Point this machine at it (writes ~/.config/strata/machine.toml, then applies)
strata init --repo ~/dotfiles --layers ""

# 3. From then on
strata edit .zshrc
strata apply
```

### New machine, existing repo

```sh
strata init git@github.com:you/dotfiles.git
```

This clones to `~/dotfiles`, asks which role layers this machine gets (e.g. `work`), writes `machine.toml`, and runs the first apply. If the machine already has dotfiles that differ from the repo, the first apply **stops and lists them** instead of overwriting — see [First apply on a machine with existing dotfiles](#first-apply-on-a-machine-with-existing-dotfiles).

---

## Core concepts

### The repo layout

```
~/dotfiles/
├── dots.toml              # repo config — every section optional (see reference below)
├── base/                  # layer: every machine gets these
│   ├── .zshrc
│   ├── .gitconfig
│   └── .config/nvim/init.lua
├── mac/                   # layer: auto-applied on macOS
│   └── .Brewfile
├── linux/                 # layer: auto-applied on any Linux…
├── arch/                  # layer: …plus your distro, read from /etc/os-release
├── windows/               # layer: auto-applied on Windows
├── work/                  # role layer: only machines that opted in at init
│   └── .gitconfig
└── home/                  # role layer: your personal machines
```

Every path inside a layer is relative to `$HOME`. `base/.config/nvim/init.lua` lands at `~/.config/nvim/init.lua`.

### Layer resolution

Layers stack in a fixed order:

```
base  →  OS layers (auto-detected)  →  role layers (in machine.toml order)
```

- macOS machine: `base → mac → <roles>`
- Arch machine: `base → linux → arch → <roles>`
- Windows machine: `base → windows → <roles>`

When two layers contain the **same path**, the later layer's file **wins whole** — no line merging, no partial overrides. If `base/.gitconfig` and `work/.gitconfig` both exist, a work machine gets exactly `work/.gitconfig`.

> **Why whole-file replace?** It keeps the mental model trivial: to know what a machine gets, find the last layer containing that path. When only a *value* differs between machines (an email, a font), don't duplicate the file — use a [variable](#configuration-reference).

Layer folders that don't exist are silently skipped, so an empty repo with just `base/` is valid, and you can add `arch/` the day you get an Arch box.

### How a machine knows who it is

`strata init` writes **one small file outside the repo** — the only per-machine state you manage:

```toml
# ~/.config/strata/machine.toml
repo = "~/dotfiles"
layers = ["work"]          # role layers; OS layers are auto-detected

[vars]
email = "you@work.example" # per-machine variable overrides
```

The repo itself is identical on every machine. Change a machine's role by editing this file.

### File statuses

Everything strata tells you is in terms of six statuses per managed file:

| Status | Meaning | What `apply` does |
|---|---|---|
| `clean` | Home matches the repo-built content | Nothing |
| `create` | File doesn't exist in home yet | Writes it |
| `update` | Repo changed; you haven't touched the home copy | Writes it |
| `drifted` | You edited the home copy; repo unchanged | **Refuses** (keep with `add`, or `--force`) |
| `conflict` | Both the repo *and* your home copy changed | **Refuses** (inspect with `diff`, then `add` or `--force`) |
| `unmanaged` | File exists but strata never wrote it (typical on first apply) | **Refuses** (adopt with `add`, or `--force`) |

---

## The TUI (bare `strata`)

Run `strata` with no subcommand and you get a full-screen, **strictly read-only** viewer (Bubble Tea + Lip Gloss). It never modifies anything — it exists to answer the questions you'd otherwise reconstruct in your head:

- **Which layer wins for every file — here, and on every other OS?**
- **What state is each file in** (clean / drifted / conflict / …)?
- **Where does every variable, hook, and permission come from?**

### Tab 1 — Layers (default)

One column per layer folder in the repo, in stack order. Each column lists the files that layer contributes. Files that a later active layer overrides are struck through with `↷ <winner>` beneath them; layers not active on this machine are dimmed; `▲` marks a file that overrides an earlier layer. A summary strip shows the resolved vars, hooks, and permission rules.

```
┌──────────┐ ┌─────────┐ ┌─────────┐ ┌──────────┐ ┌──────────┐
│  base ✓  │ │  mac ✓  │ │  linux  │ │ windows  │ │  work ✓  │
└──────────┘ └─────────┘ └─────────┘ └──────────┘ └──────────┘
.zshrc        .Brewfile ⚙  alacritty…  .wslconfig   .gitconfig ▲ {{ }}
.gitconfig                                          .ssh/config 600
  ↷ work
nvim/init.lua
```

### Tab 2 — Files

One row per file (the union of what *every* OS would manage), showing the winning layer on this machine, the winner on mac/linux/windows, and the live status:

```
FILE                         WINS HERE  MAC    LINUX  WIN      STATUS
.Brewfile ⚙                  mac        mac    —      —       ● clean
.gitconfig {{ }}             work       work   work   work    ~ drifted
.ssh/config 600              work       work   work   work    ● clean
.wslconfig                   n/a        —      —      windows       —
```

Badges: `{{ }}` = variable-substituted · `⚙` = has a hook · `600` = explicit permission rule.

### Tab 3 — Vars & Rules

Every variable with its value **on this machine**, where it came from (`machine.toml` overrides show the dots.toml default struck through), which substituted files use which vars, plus the hook and permission tables.

### Drilldown

`enter` on any file opens a modal with the full story: source path → destination, what it overrides (`overrides base/.gitconfig`), how it resolves on each OS, the substituted variable values and their origins, permissions/hook/last-applied hash — and for non-clean files, a diff excerpt (`d` for the full scrollable diff).

### Keys

| Key | Action |
|---|---|
| `←` `→` or `1` `2` `3` | switch tabs (wraps) |
| `↑` `↓` | move selection (Files tab) / scroll (full diff) |
| `enter` | open drilldown for the selected file |
| `d` | full diff (inside the drilldown) |
| `esc` | close drilldown / diff |
| `q` / `ctrl+c` | quit |

If the terminal is too narrow, the per-OS columns drop first. The design comps and handoff spec the TUI was built from live in [docs/design/tui/](docs/design/tui/).

---

## Command reference

### `strata apply`

Builds every managed file (stack layers → substitute variables → resolve permissions) and copies the ones that changed into `$HOME`, then runs hooks for changed files.

```
$ strata apply
wrote .gitconfig
wrote .zshrc
hook [.Brewfile]: brew bundle --file=~/.Brewfile
```

- `--dry-run` / `-n` — print what would be written, write nothing
- `--force` — also overwrite `drifted` / `conflict` / `unmanaged` files (take the repo's version)

If **any** file is drifted/conflicted/unmanaged and `--force` isn't given, apply writes **nothing at all** — it's all-or-nothing, so a half-applied state can't happen:

```
$ strata apply
error: refusing to overwrite local changes:
  drifted   .zshrc
keep your version with 'strata add <file>', or overwrite with 'strata apply --force'
```

Re-running apply when everything is clean prints `nothing to do` — it's always safe to run.

### `strata status`

One line per file that needs attention; silent about clean files.

```
$ strata status
update    .gitconfig
drifted   .zshrc
```

```
$ strata status        # when everything matches
clean: 14 files up to date
```

### `strata diff`

Unified diff of every non-clean file: `home/<file>` (what's on disk now) against `repo/<file>` (what apply would write). Because it compares in both directions, edits you made directly in `$HOME` show up too — as lines apply would *remove*:

```
$ strata diff
--- home/.zshrc (drifted)
+++ repo/.zshrc
@@ -1,3 +1,2 @@
 hello
-local edit
```

### `strata edit <file>`

Opens the **winning layer's source file** in `$EDITOR` (falls back to `vi`), then shows the diff and offers to apply:

```
$ strata edit .gitconfig      # on a work machine, opens ~/dotfiles/work/.gitconfig
... editor session ...
--- home/.gitconfig (update)
+++ repo/.gitconfig
@@ ...
apply now? [y/N] y
wrote .gitconfig
```

You never have to remember which layer wins — `edit` resolves it exactly like `apply` does. To edit a *non-winning* copy (say `base/.gitconfig` while `work/` overrides it), just open that file in your editor directly; it's a plain file.

If the file isn't managed yet: `error: .foorc is not managed (try: strata add .foorc)`.

### `strata add <file> [--layer <name>]`

Copies a file **from `$HOME` into the repo**. One command for two jobs:

- **Adopt** a file strata doesn't manage yet: `strata add .vimrc` → `base/.vimrc`
- **Absorb** edits you made directly in `$HOME` on a managed file: the drifted content becomes the repo content, and the file reads `clean` again

```
$ strata add .zshrc
added .zshrc → base/.zshrc
```

Path forms all work: `strata add .zshrc`, `strata add ~/.zshrc`, `strata add /Users/you/.config/foo`. Files outside your home directory are rejected.

- `--layer mac` — put the file in a specific layer instead of the default (the currently-winning layer, or `base` for new files)
- If the file is on the `substitute` list, add warns you: the copy you just captured contains the **expanded** values, so re-insert the `{{tokens}}` by hand afterwards (`strata edit <file>`).

### `strata init [git-url]`

First-time setup on a machine.

```sh
strata init git@github.com:you/dotfiles.git   # clone to ~/dotfiles (--dir to change), prompt for role layers, write machine.toml, first apply
strata init --repo ~/dotfiles --layers work   # use an existing local repo, skip the prompt
strata init --repo ~/dotfiles --layers ""     # no role layers
```

### `strata sync`

`git pull --ff-only` in the repo, then `apply`. The "give me my other machine's latest changes" command.

---

## Configuration reference

### `dots.toml` (repo root — every section optional)

A repo with no `dots.toml` at all is valid: every file is copied byte-for-byte with default permissions.

```toml
# ── Substitution opt-in ─────────────────────────────────────────────
# ONLY these files get {{var}} tokens replaced. Everything else is copied
# byte-for-byte, so shell ${VARS}, other tools' {{ }} syntax, etc. are
# never touched. (TOML note: this top-level key must appear BEFORE any
# [section].)
substitute = [".gitconfig", ".Brewfile"]

# ── Variable defaults ───────────────────────────────────────────────
# Overridden per machine by machine.toml [vars].
[vars]
email = "personal@example.com"
name  = "Your Name"

# ── Permissions ─────────────────────────────────────────────────────
# glob → octal mode. `**` crosses directories. When several patterns
# match, the LONGEST pattern wins. Files with no match: 644, or 755 if
# the repo copy is executable. (git only stores the exec bit, which is
# why .ssh needs this section.)
[permissions]
".ssh/**" = "600"

# ── Hooks ───────────────────────────────────────────────────────────
# After a successful apply, if the keyed file was among those written,
# run the command (sh -c on Unix, cmd /C on Windows).
[hooks]
".Brewfile" = "brew bundle --file=~/.Brewfile"
```

Substitution tokens look like `{{email}}` (spaces allowed: `{{ email }}`; names are `[A-Za-z0-9_]`). An **undefined variable in a substituted file fails the whole apply** — strata never writes a half-substituted config:

```
error: .gitconfig: undefined variables: email
```

### `machine.toml` (`~/.config/strata/machine.toml`)

```toml
repo = "~/dotfiles"        # ~ is expanded
layers = ["work"]          # role layers, applied in this order after OS layers

[vars]                     # overrides dots.toml [vars] key-by-key
email = "you@work.example"
```

### State file (`~/.local/state/strata/state.json`)

Maintained automatically — you never edit it. It maps each managed file to the SHA-256 of what strata last wrote, which is what powers drift detection. Deleting it is safe but demotes every existing file to `unmanaged` on the next apply (strata will ask before overwriting them again).

---

## Everyday workflows

### Change a setting

```sh
strata edit .zshrc        # edit source → see diff → y → applied
```

Or edit `~/dotfiles/base/.zshrc` in your IDE and run `strata apply`. Same thing.

### "I edited ~/.zshrc directly" (drift)

You will. It's fine — nothing is lost:

```sh
$ strata status
drifted   .zshrc
$ strata diff              # see exactly what you changed
$ strata add .zshrc        # keep your edit: absorb it into the repo
#   …or…
$ strata apply --force     # discard your edit: take the repo's version
```

### Both changed (conflict)

`status` says `conflict` when you edited the home copy *and* the repo version moved (e.g. after `git pull`). Look at `strata diff`, then pick a side: `strata add <file>` keeps your local content (hand-merge the repo's changes into it if you want both), `strata apply --force` takes the repo's.

### Machine-specific file

```sh
mkdir -p ~/dotfiles/mac
cp ~/.Brewfile ~/dotfiles/mac/.Brewfile      # or: strata add .Brewfile --layer mac
```

Only macOS machines will get it. Same idea for `work/`, `arch/`, etc.

### One line differs per machine

Don't duplicate the file into a layer — use a variable:

```toml
# dots.toml
substitute = [".gitconfig"]
[vars]
email = "personal@example.com"
```

```ini
# base/.gitconfig
[user]
    email = {{email}}
```

```toml
# machine.toml on the work laptop
[vars]
email = "you@work.example"
```

### Propagate changes to your other machines

```sh
# machine A: edit, apply, then commit & push the repo with git as usual
# machine B:
strata sync                # git pull --ff-only + apply
```

strata doesn't wrap git beyond `sync` — your dotfiles repo is a normal git repo; use git however you like.

### First apply on a machine with existing dotfiles

Every real machine already has a `~/.zshrc`. strata will not silently destroy it:

```
$ strata apply
error: refusing to overwrite local changes:
  unmanaged .zshrc
  unmanaged .gitconfig
keep your version with 'strata add <file>', or overwrite with 'strata apply --force'
```

Go file-by-file: `strata diff` to compare, then `strata add .zshrc` for the ones where the machine's version is the keeper, and finish with `strata apply --force` to take the repo's version of the rest. (Files whose content already matches the repo are adopted silently.)

---

## How the safety model works

For each managed file, strata compares three things:

```
desired  = layers stacked + variables substituted     (what the repo says)
current  = the file in $HOME right now
last     = SHA-256 of what strata last wrote           (state file)
```

- `current == desired` → **clean**
- file missing → **create**
- no `last` recorded → **unmanaged** (strata never wrote this file)
- `current == last` (home untouched) → **update**
- `desired == last` (repo unchanged) → **drifted** (only you moved)
- all three differ → **conflict**

Guarantees built on top of that:

- **All-or-nothing apply.** If anything would be refused, nothing is written.
- **Atomic writes.** Files are written to a temp file and `rename()`d into place; a crash mid-apply can't leave a truncated `.zshrc`.
- **Hooks run last**, only after every file write succeeded, and only for files that actually changed.
- **Fail-loud substitution.** An undefined `{{var}}` aborts the apply before anything is written.

---

## Environment variables

Mainly for testing and scripting — normally you never set these:

| Variable | Overrides | Default |
|---|---|---|
| `STRATA_HOME` | Target "home" directory | your real home dir |
| `STRATA_CONFIG` | Path to `machine.toml` | `~/.config/strata/machine.toml` |
| `STRATA_STATE` | Path to the state file | `~/.local/state/strata/state.json` |
| `EDITOR` | Editor used by `strata edit` | `vi` |

These make it trivial to point strata at a sandbox and try anything risk-free:

```sh
STRATA_HOME=/tmp/fakehome STRATA_CONFIG=/tmp/m.toml STRATA_STATE=/tmp/s.json strata apply
```

---

## Building and testing

```sh
go build -o strata .        # build
go test ./...               # unit + end-to-end tests (all run in temp dirs)
go vet ./... && gofmt -l .  # lint/format check
```

Code layout:

```
main.go, cmd_*.go        # cobra CLI; one file per command (cmd_tui.go = bare-strata launcher)
internal/config/         # dots.toml + machine.toml parsing, var merging
internal/layers/         # OS detection (/etc/os-release on Linux) + layer resolution
internal/subst/          # {{var}} substitution, fail-loud on undefined
internal/perms/          # permission rules (doublestar globs, longest match wins)
internal/state/          # last-applied hash store
internal/engine/         # Plan (status classification), Apply, RunHooks
internal/fsutil/         # SHA-256 hashing, atomic writes
internal/tui/            # read-only TUI (Bubble Tea + Lip Gloss); design: docs/design/tui/
```

The engine takes `GOOS` and the os-release content as *parameters*, so tests exercise mac/arch/windows behavior on any platform.

---

## Security note

`[hooks]` commands are executed verbatim through the shell on apply — deliberately, exactly like git hooks or a Makefile. The trust boundary is the repo itself: only `init`/`apply` dotfiles repos you trust, because *any* dotfiles repo is arbitrary code execution by definition (it controls your `.zshrc`).

strata never reads or writes anything outside `$HOME` (targets), your repo (sources), and its two config/state files.

---

## What strata deliberately doesn't do (yet)

Kept out of v1 to keep the model small:

- **Secrets/encryption** — keep private keys out of the repo (or encrypt them with a dedicated tool)
- **File removal tracking** — deleting a file from the repo doesn't delete it from `$HOME`
- **`run_once` scripts, symlink mode, partial file merging, non-`$HOME` targets** (e.g. `/etc`)

All addable later without changing the core model.
