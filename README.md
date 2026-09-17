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

**Release build** (macOS or Linux — no Go, no git clone):

```sh
curl -fsSL https://raw.githubusercontent.com/D1srupt3d/strata/main/get.sh | sh
```

`get.sh` downloads the latest release, checks its signature (made by the strata release key) and its SHA-256, makes sure the binary runs and reports that exact version, and installs `~/.local/bin/strata` — refusing to install anything if a check fails. The signature check uses `ssh-keygen -Y`, which needs OpenSSH 8.1 or newer (`ssh -V` shows yours; any macOS or Linux from the last few years has it). If `~/.local/bin` isn't on your `PATH` it adds it to your login profile, exactly like `install.sh`. From then on, update with [`strata upgrade`](#strata-upgrade). Re-running `get.sh` reinstalls the latest release. On Windows, download the `.zip` from the [releases page](https://github.com/D1srupt3d/strata/releases).

**From source** (for hacking on strata):

```sh
sh install.sh
```

That builds the binary, copies it to `~/.local/bin/strata`, and — if that directory isn't on your `PATH` — appends one `export PATH=...` line to your login profile (`~/.zprofile` for zsh; `~/.bash_profile` if it exists, else `~/.profile`). It's idempotent: re-run it any time to upgrade; it never adds the line twice. `STRATA_BIN_DIR=/somewhere sh install.sh` overrides the destination.

> The installer deliberately never edits `.zshrc`/`.bashrc`: with strata those are managed dotfiles, and an installer edit there would show up as local drift that blocks your first apply. Putting `export PATH="$HOME/.local/bin:$PATH"` in your repo's `base/.zshrc` is still the tidiest option — then the installer finds it on your `PATH` and touches nothing.

Manual alternative: `go build -o strata .` and move the binary anywhere on your `PATH`. On Windows: `go build -o strata.exe .` and place it in a directory listed in your `Path` environment variable.

Cross-compiles with plain Go: `GOOS=linux go build`, `GOOS=windows go build`, etc.

To reverse all of this later, run [`strata uninstall`](#strata-uninstall) — it removes the binary, config, state, and the PATH line, leaving your actual dotfiles alone.

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

OS layer folders that don't exist are skipped, so an empty repo with just `base/` is valid, and you can add `arch/` the day you get an Arch box. A **role** layer you list in `machine.toml` is different: it must be a folder in the repo. Skipping a typo like `wrok` would drop every file `work/` provides — and the next apply would delete them from `$HOME` — so strata stops with an error instead.

### How a machine knows who it is

`strata init` writes **one small file outside the repo** — the only per-machine state you manage:

```toml
# ~/.config/strata/machine.toml
repo = "~/dotfiles"        # full path (or ~/...); a relative path is an error
layers = ["work"]          # role layers; OS layers are auto-detected

[vars]
email = "you@work.example" # per-machine variable overrides
```

The repo itself is identical on every machine. Change a machine's role by editing this file.

### File statuses

Everything strata tells you is in terms of eight statuses per managed file:

| Status | Meaning | What `apply` does |
|---|---|---|
| `clean` | Home matches the repo-built content | Nothing |
| `create` | File doesn't exist in home yet | Writes it |
| `update` | Repo changed; you haven't touched the home copy | Writes it |
| `drifted` | You edited the home copy; repo unchanged | **Refuses** (keep with `add`, or `--force`) |
| `conflict` | Both the repo *and* your home copy changed | **Refuses** (inspect with `diff`, then `add` or `--force`) |
| `unmanaged` | File exists but strata never wrote it (typical on first apply) | **Refuses** (adopt with `add`, or `--force`) |
| `removed` | strata wrote it before, but no layer provides it anymore | Deletes it from home (**refuses** if you edited it since the last apply, unless `--force`) |
| `chmod` | Content matches, but an explicit `[permissions]` rule — or the repo copy's executable bit — disagrees with the file's mode | Fixes the mode only: no rewrite, no hook. (With no rule, a mode you tightened by hand is left alone. Never reported on Windows.) |

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

If the terminal is too narrow, the per-OS columns drop first.

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

- `--dry-run` / `-n` — show exactly what apply would do, file by file (`would write`, `would chmod`, `would remove`, `would hook`, or `blocked` with the reason and the way out), and write nothing. If anything is blocked it says so: apply is all-or-nothing, so one blocked file means nothing gets written.
- `--force` — also overwrite `drifted` / `conflict` / `unmanaged` files (take the repo's version)

If **any** file is drifted/conflicted/unmanaged and `--force` isn't given, apply writes **nothing at all** — it's all-or-nothing:

```
$ strata apply
error: refusing to overwrite local changes:
  drifted   .zshrc
keep your version with 'strata add <file>', or overwrite with 'strata apply --force'
```

If a write itself fails partway — a full disk, a folder you can't write to — the files already written stay written: they're recorded, and their hooks queued, so the next `apply` picks up where this one stopped.

Re-running apply when everything is clean prints `nothing to do` — it's always safe to run.

Hooks run in `$HOME`, after every write has succeeded. A hook that fails (or is interrupted) stays **pending** in the state file: `status` lists it, and the next `apply` retries it — even though its file already reads clean — until it succeeds. Hooks have no time limit, on purpose: a first `brew bundle` can take an hour. Ctrl-C stops one; it stays pending and reruns on the next apply. (While a hook runs, other strata commands that change state stop with `another strata is already running`.)

A `$HOME` dotfile that is a **symlink** (into Dropbox, another tool's folder, …) is never silently replaced: apply refuses it like a drifted file, and `--force` swaps in a regular file.

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

A hook that failed last time shows up as `hook      .Brewfile (pending: …)` until an apply retries it successfully.

Exit status is `0` when everything is clean and `1` when anything needs attention (no `error:` line — it's an answer, not a failure), so it composes in scripts and prompts: `strata status || strata diff`.

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

Opens the **winning layer's source file** in your editor — `$VISUAL`, else `$EDITOR`, else `vi` — then shows the diff and offers to apply. The editor setting may include arguments; GUI editors need their wait flag (`EDITOR="code --wait"`), or strata shows the diff before you've made your edit.

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

- `--layer mac` — put the file in a specific layer instead of the default (the currently-winning layer, or `base` for new files). The layer must be a folder in the repo or one of this machine's layers, so a typo like `--layer wrok` is an error rather than a new layer. If that layer isn't the one this machine gets the file from — a later layer overrides it, or it isn't one of this machine's layers — the copy is saved there but `$HOME` is left alone, and add tells you which layer wins. If the repo can't be planned right now (say, an undefined `{{var}}`), `add` without `--layer` refuses rather than guess `base` — which could push a work-only file to every machine. Fix the error, or name the layer.
- If the file is on the `substitute` list, add warns you: the copy you just captured contains the **expanded** values, so re-insert the `{{tokens}}` by hand afterwards (`strata edit <file>`).

### `strata init [git-url | local-repo]`

First-time setup on a machine.

```sh
strata init git@github.com:you/dotfiles.git   # clone to ~/dotfiles (--dir to change), prompt for role layers, write machine.toml, first apply
strata init ~/src/dotfiles                    # existing local repo: used in place, not cloned
strata init --repo ~/dotfiles --layers work   # use an existing local repo, skip the prompt
strata init --repo ~/dotfiles --layers ""     # no role layers
```

`--layers ""` means "no role layers" and skips the prompt. After writing `machine.toml`, init warns if the repo isn't a git clone (apply works, but `strata sync` needs `git pull`) and lists every `[vars]` entry still on its `dots.toml` default — override any of them under `[vars]` in `machine.toml`.

Re-running init (after moving the repo, say) replaces `repo` and `layers` but **keeps this machine's `[vars]` overrides** — comments in the old file aren't kept. A `machine.toml` it can't parse is refused rather than overwritten, and a role layer with no folder in the repo is rejected before anything is written.

### `strata sync`

`git pull --ff-only` in the repo, then `apply`. The "give me my other machine's latest changes" command.

### `strata rm <file>`

Deletes the file from its **winning layer**, then applies. If no other layer provides the file, apply removes it from `$HOME` too (with the usual refuse-if-you-edited-it safety). If an earlier layer still provides it, that layer wins again and the home copy is rewritten:

```
$ strata rm .tmux.conf        # sole provider → gone from repo AND $HOME
deleted ~/dotfiles/base/.tmux.conf
removed .tmux.conf

$ strata rm .gitconfig        # work/ override removed → base/ wins again
deleted ~/dotfiles/work/.gitconfig
wrote .gitconfig
```

Deleting a layer file by hand (or via `git rm` + `sync` on another machine) works identically — `status` shows the orphan as `removed` and the next `apply` cleans it up.

If apply would refuse right now (a drifted or conflicting file anywhere), `rm` refuses **before** deleting anything, so it never stops half done.

### `strata upgrade`

Replaces the running strata with the latest signed release — no git clone, no `git pull`. strata never touches the network unless you run this (or `get.sh`).

```
$ strata upgrade
upgraded strata 2026.9.1 → 2026.9.2

$ strata upgrade --check      # exit 1 if a newer release exists
update available: 2026.9.1 → 2026.9.2 (run 'strata upgrade')
```

Nothing is replaced until every check passes, in order: the release is newer than this build (never a downgrade), `checksums.txt` carries a valid signature from the strata release key (an SSH key; `ssh-keygen -Y` format, namespace `strata-release`), the signed checksums list this platform's archive by its exact versioned name, the archive's SHA-256 matches, and the new binary runs and reports that version. Then it's swapped in atomically (on Windows the running `.exe` is moved aside to `strata.exe.old` and cleaned up next time). If anything fails, your installed strata is left exactly as it was.

- `--check` — only report whether a newer release exists
- `--force` — reinstall even if you're on the latest release
- `update` works as an alias

Only **release builds** (from GitHub releases or `get.sh`) upgrade themselves. A Homebrew install defers to `brew upgrade strata`, and a binary you built from source (`install.sh`, `go build`) is yours to update: `git pull && sh install.sh` — or switch to release builds with `get.sh`. Releases published before signing began (up to v2026.9.0) can't be installed this way.

### `strata uninstall`

Removes strata itself — the binary, `~/.config/strata/machine.toml`, `~/.local/state/strata/state.json` (and its lock file), and the `export PATH` line `install.sh` added to your shell profile. It **does not** touch the dotfiles strata copied into `$HOME` (those are ordinary files now) or your dotfiles repo — delete those yourself if you want them gone.

```
$ strata uninstall
This will remove:
  ~/.local/state/strata/state.json
  ~/.config/strata/machine.toml
  ~/.local/bin/strata
  PATH line in ~/.zprofile

Your dotfiles in $HOME and your dotfiles repo are left untouched.

Remove these? [y/N]: y
removed ~/.local/state/strata/state.json
removed ~/.config/strata/machine.toml
removed ~/.local/bin/strata
cleaned PATH line from ~/.zprofile

strata uninstalled. Open a new terminal (or run 'hash -r') to clear the cached command.
```

- `--dry-run` / `-n` — list what would be removed, remove nothing
- `--yes` / `-y` — skip the confirmation prompt

Only the files that actually exist are listed and removed, so it's safe to re-run (a second run prints `nothing to remove`). The binary is deleted last, so if a step fails partway you still have a working `strata` to retry with.

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

# ── Ignore ──────────────────────────────────────────────────────────
# Paths inside a layer that are NOT dotfiles: editor scratch, caches,
# and app-managed state that rewrites itself. Same glob rules as
# [permissions]. Ignoring is not removing — a file you ignore after it
# was already applied is simply forgotten, and its $HOME copy is left
# alone. (Also a top-level key: must appear BEFORE any [section].)
ignore = [".claude/settings.json", "**/*.log"]

# ── Variable defaults ───────────────────────────────────────────────
# Overridden per machine by machine.toml [vars].
[vars]
email = "personal@example.com"
name  = "Your Name"

# ── Permissions ─────────────────────────────────────────────────────
# glob → octal mode. `**` crosses directories. When several patterns
# match, the LONGEST (most specific) pattern wins; two equally long
# matches that disagree are an error, never a coin flip. Files with no
# match: 644, or 755 if the repo copy is executable. A rule is enforced on
# existing files too (status `chmod`). Modes are 000–777 (no setuid,
# setgid or sticky). Folders strata creates are as private as the file
# that needed them (700 for a 600 file, else 755); existing folders are
# never changed. (git only stores the exec bit, which is why .ssh needs
# this section.)
[permissions]
".ssh/**" = "600"

# ── Hooks ───────────────────────────────────────────────────────────
# After apply writes the keyed file, run the command in $HOME (sh -c on
# Unix, cmd /C on Windows). A hook that fails stays pending and is
# retried by the next apply until it succeeds.
[hooks]
".Brewfile" = "brew bundle --file=~/.Brewfile"
```

These patterns are **always** ignored, on every OS, without configuring anything — they are written into layer dirs by the OS file browser rather than by you, and mean nothing on another machine:

```
**/.DS_Store   **/._*   **/.Spotlight-V100   **/Thumbs.db   **/desktop.ini
```

A malformed ignore pattern is an error, not a silent non-match, so a typo can never quietly manage a file you meant to exclude. Unknown keys are errors too: a typo like `[hook]` for `[hooks]` in `dots.toml` (or `layer =` for `layers =` in `machine.toml`) fails loudly instead of silently doing nothing. The same goes for a `repo` in `machine.toml` that isn't a full path (strata would otherwise depend on which folder you ran it from) and for a role layer with no folder.

Substitution tokens look like `{{email}}` (spaces allowed: `{{ email }}`; names are `[A-Za-z0-9_]`). An **undefined variable in a substituted file fails the whole apply** — strata never writes a half-substituted config:

```
error: .gitconfig: undefined variables: email
```

### `machine.toml` (`~/.config/strata/machine.toml`)

```toml
repo = "~/dotfiles"        # full path, ~ expanded (relative is an error). Moving it? See "Move the repo to a new folder"
layers = ["work"]          # role layers, applied in this order after OS layers

[vars]                     # overrides dots.toml [vars] key-by-key
email = "you@work.example"
```

### State file (`~/.local/state/strata/state.json`)

Maintained automatically — you never edit it. It maps each managed file to the SHA-256 of what strata last wrote (which is what powers drift detection), plus any hooks still pending a retry. It carries a format `version`, so a strata older than the file refuses it rather than misreading it. Commands that change it take a lock (`state.json.lock`, released automatically if the process dies), so two strata runs can't clobber each other's updates. Deleting it is safe but demotes every existing file to `unmanaged` on the next apply (strata will ask before overwriting them again). An entry pointing outside `$HOME` (hand-edited or corrupted) is refused, never deleted.

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

### Move the repo to a new folder

Move it, then point `machine.toml` at the new path — that's the whole migration:

```sh
mv ~/dotfiles ~/code/dotfiles
# edit ~/.config/strata/machine.toml:  repo = "~/code/dotfiles"
strata status              # should say clean
```

The state file tracks files by their path in `$HOME`, not in the repo, so everything stays managed and nothing is rewritten. No re-init needed.

**Order matters.** strata treats a missing repo folder as a repo with no files: every managed file shows as `removed`, and the next `apply` deletes them from `$HOME` (with the usual refuse-if-you-edited-it safety). Update `repo` *before* running `apply`.

That's also a deliberate way to clear out everything strata manages — e.g. to start clean before `strata init` against a new location. Check `strata status` first so you know exactly what will go.

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

- `current == desired` → **clean** (or **chmod**, if an explicit permission rule — or the repo's exec bit — disagrees with the file's mode)
- file missing → **create**
- no `last` recorded → **unmanaged** (strata never wrote this file)
- `current == last` (home untouched) → **update**
- `desired == last` (repo unchanged) → **drifted** (only you moved)
- all three differ → **conflict**

Guarantees built on top of that:

- **All-or-nothing apply.** If anything would be refused — including replacing a symlink you set up — nothing is written. (A write that fails for another reason partway, like a full disk, keeps what was already written recorded, with its hooks queued.)
- **Atomic, durable writes.** Files are written to a temp file, flushed to disk, and `rename()`d into place; neither a crash nor a power cut mid-apply can leave a truncated `.zshrc`.
- **Hooks run last, in `$HOME`**, only after every file write succeeded and only for files that actually changed — and a failed hook is retried on the next apply until it succeeds.
- **Fail-loud config.** An undefined `{{var}}`, an unknown config key, a relative `repo`, a role layer with no folder, a mode outside 000–777, or two equally specific permission rules that disagree abort the apply before anything is written.

---

## Environment variables

Mainly for testing and scripting — normally you never set these:

| Variable | Overrides | Default |
|---|---|---|
| `STRATA_HOME` | Target "home" directory | your real home dir |
| `STRATA_CONFIG` | Path to `machine.toml` | `~/.config/strata/machine.toml` |
| `STRATA_STATE` | Path to the state file | `~/.local/state/strata/state.json` |
| `STRATA_BIN` | Path to the strata binary `uninstall` deletes | the running binary (`os.Executable()`) |
| `VISUAL` / `EDITOR` | Editor used by `strata edit` (`VISUAL` wins; may include arguments, e.g. `code --wait`; on Windows, double-quote a path with spaces) | `vi` |
| `STRATA_RELEASE_API` | Release endpoint `strata upgrade` and `get.sh` query (tests, mirrors) | GitHub's `releases/latest` for D1srupt3d/strata |

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
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...   # linter (CI runs it)
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...    # known-vulnerability scan (CI runs it)
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
internal/tui/            # read-only TUI (Bubble Tea + Lip Gloss)
```

The engine takes `GOOS` and the os-release content as *parameters*, so tests exercise mac/arch/windows behavior on any platform.

---

## Security note

**Releases are signed.** Release CI signs `checksums.txt` with a dedicated SSH key held in a GitHub environment that only `v*` tag runs can use; `strata upgrade` and `get.sh` verify that signature against the public key built into strata (`internal/release/release_key.pub`, fingerprint `SHA256:nMQXiQxd18neATjd15cvS8DQ5ihMQXbrgBa6xLKedNY`) before trusting any download. Every release also carries GitHub build provenance: `gh attestation verify <file> --repo D1srupt3d/strata`.

`[hooks]` commands are executed verbatim through the shell on apply — deliberately, exactly like git hooks or a Makefile. The trust boundary is the repo itself: only `init`/`apply` dotfiles repos you trust, because *any* dotfiles repo is arbitrary code execution by definition (it controls your `.zshrc`).

strata never reads or writes anything outside `$HOME` (targets), your repo (sources), and its two config/state files.

---

## What strata deliberately doesn't do (yet)

Kept out of v1 to keep the model small:

- **Secrets/encryption** — keep private keys out of the repo (or encrypt them with a dedicated tool)
- **`run_once` scripts, symlink mode, partial file merging, non-`$HOME` targets** (e.g. `/etc`)

All addable later without changing the core model.
