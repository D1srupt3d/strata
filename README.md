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

Run bare `strata` for a read-only [terminal UI](#the-tui) showing which layer wins for every file, on every OS.

## Install

macOS or Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/D1srupt3d/strata/main/get.sh | sh
```

This installs `~/.local/bin/strata`, but only after checking the release's signature, its SHA-256, and that the binary runs and reports the right version. Update later with `strata upgrade`. On Windows, download the `.zip` from the [releases page](https://github.com/D1srupt3d/strata/releases).

To remove strata later, run `strata uninstall`: it deletes the binary, its config and state, and the PATH line — never your dotfiles.

<details>
<summary>Building from source, and what the installers change</summary>

**From source:** `sh install.sh` builds the binary and copies it to `~/.local/bin/strata` (`STRATA_BIN_DIR=/somewhere` changes that). Re-run it any time to update. Or build by hand with `go build -o strata .` (`strata.exe` on Windows) and put the binary anywhere on your `PATH`. Cross-compiling is plain Go: `GOOS=linux go build`.

**PATH:** if `~/.local/bin` isn't on your `PATH`, `get.sh` and `install.sh` append one `export PATH=...` line to your login profile (`~/.zprofile` for zsh; `~/.bash_profile` if it exists, else `~/.profile`), and never twice. They deliberately never edit `.zshrc`/`.bashrc`: strata manages those, and an installer edit would show up as drift that blocks your first apply. The tidiest setup is putting `export PATH="$HOME/.local/bin:$PATH"` in your repo's `base/.zshrc` — then the installers find it and touch nothing.

**Signature check:** `get.sh` uses `ssh-keygen -Y`, which needs OpenSSH 8.1 or newer (`ssh -V`; any macOS or Linux from the last few years has it).

</details>

## Try it without touching your dotfiles

[**strata-dots**](https://github.com/D1srupt3d/strata-dots) is a small template repo with one example of each feature — layers, a role layer, variables, permissions and a hook. Its `try.sh` applies it to a throwaway home folder:

```sh
git clone https://github.com/D1srupt3d/strata-dots.git && cd strata-dots
./try.sh            # set up a sandbox home in ./.sandbox/ and apply
./try.sh            # open the TUI
./try.sh status     # or any other strata command
```

When you're ready, click **Use this template** on GitHub to make your own copy, then follow [New machine, existing repo](#new-machine-existing-repo).

## Get started

### From scratch

```sh
mkdir ~/dotfiles && git -C ~/dotfiles init       # your dotfiles repo is a plain git repo
strata init --repo ~/dotfiles --layers ""        # point this machine at it
strata add .zshrc                                # copy ~/.zshrc into base/
strata status                                    # clean: 1 files up to date
```

Commit and push with git as usual. Add more files with `strata add`, and a `dots.toml` once you want [variables, permissions or hooks](#configuration).

### New machine, existing repo

```sh
strata init git@github.com:you/dotfiles.git
```

This clones to `~/dotfiles`, asks which role layers this machine gets (e.g. `work`), writes `machine.toml`, and runs the first apply. Existing dotfiles that differ from the repo are never overwritten — see [First apply on a machine with existing dotfiles](#first-apply-on-a-machine-with-existing-dotfiles).

## How it works

### The repo mirrors `$HOME`

```
~/dotfiles/
├── dots.toml              # optional repo config
├── base/                  # every machine gets these
│   ├── .zshrc
│   ├── .gitconfig
│   └── .config/nvim/init.lua
├── mac/                   # auto-applied on macOS
├── linux/                 # auto-applied on any Linux…
├── arch/                  # …plus your distro, read from /etc/os-release
├── windows/               # auto-applied on Windows
└── work/                  # role layer: only machines that opted in
    └── .gitconfig
```

Every path inside a layer is relative to `$HOME`: `base/.config/nvim/init.lua` lands at `~/.config/nvim/init.lua`.

### Layers stack, and the last one wins

```
base  →  OS layers (auto-detected)  →  role layers (in machine.toml order)
```

A Mac gets `base → mac → <roles>`; an Arch box gets `base → linux → arch → <roles>`.

When two layers contain the same path, the later layer's file **wins whole** — no line merging. A work machine with both `base/.gitconfig` and `work/.gitconfig` gets exactly `work/.gitconfig`. When only a *value* differs between machines (an email, a font), don't copy the file into a layer: use a [variable](#one-line-differs-per-machine).

OS layer folders are optional — add `arch/` the day you get an Arch box. Role layers are not: a typo like `wrok` in `machine.toml` is an error, because silently skipping it would make apply delete every file `work/` provides.

### Each machine has one small config file

`strata init` writes `~/.config/strata/machine.toml`, the only per-machine setting. The repo itself is identical everywhere.

```toml
repo = "~/dotfiles"        # full path (or ~/...); a relative path is an error
layers = ["work"]          # role layers; OS layers are auto-detected

[vars]
email = "you@work.example" # per-machine variable overrides
```

### File statuses

strata compares three versions of every file: what the repo builds, what's in `$HOME` now, and the hash of what strata last wrote. That gives eight statuses:

| Status | Meaning | What `apply` does |
|---|---|---|
| `clean` | Home matches the repo | Nothing |
| `create` | Not in home yet | Writes it |
| `update` | Repo changed; home copy untouched | Writes it |
| `drifted` | You edited the home copy; repo unchanged | **Refuses** — keep it with `add`, or `--force` |
| `conflict` | Both the repo and your home copy changed | **Refuses** — inspect with `diff`, then `add` or `--force` |
| `unmanaged` | Exists, but strata never wrote it (typical on first apply) | **Refuses** — adopt with `add`, or `--force` |
| `removed` | strata wrote it, but no layer provides it anymore | Deletes it (refuses if you edited it since) |
| `chmod` | Content matches, but the mode doesn't match a `[permissions]` rule or the repo's exec bit | Fixes the mode only (never on Windows) |

What strata guarantees:

- **All-or-nothing.** If any file would be refused, apply writes nothing.
- **Crash-safe writes.** Files go to a temp file, get flushed to disk, then renamed into place — a power cut can't leave half a `.zshrc`.
- **Hooks run last**, only for files that changed, and a failed hook is retried on the next apply.
- **Config errors stop everything.** An undefined `{{var}}`, an unknown key, or a bad pattern aborts before anything is written.

## Everyday workflows

### Change a setting

```sh
strata edit .zshrc        # edit the source → see the diff → y → applied
```

Or edit `~/dotfiles/base/.zshrc` in your editor and run `strata apply`.

### You edited a file in `$HOME` (drift)

Nothing is lost:

```sh
strata status              # drifted   .zshrc
strata diff                # see exactly what you changed
strata add .zshrc          # keep your edit: copy it into the repo
strata apply --force       # …or discard it and take the repo's version
```

`conflict` works the same way: it means the repo *also* changed (say, after a `git pull`). Check `strata diff`, then pick a side.

### A file only some machines get

```sh
strata add .Brewfile --layer mac     # only macOS machines get it
```

Same idea for `work/`, `arch/`, and any other layer.

### One line differs per machine

Use a variable instead of copying the file into a layer:

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

### Sync your other machines

Commit and push on one machine, then on the others:

```sh
strata sync                # git pull --ff-only, then apply
```

strata doesn't wrap git beyond `sync` — use git however you like.

### First apply on a machine with existing dotfiles

Every real machine already has a `~/.zshrc`, and strata won't silently replace it:

```
$ strata apply
error: refusing to overwrite local changes:
  unmanaged .zshrc
  unmanaged .gitconfig
keep your version with 'strata add <file>', or overwrite with 'strata apply --force'
```

Go file by file: `strata diff` to compare, `strata add` the ones where this machine's version is the keeper, then `strata apply --force` for the rest. Files that already match the repo are adopted automatically.

### Move the repo

Move the folder, then update `repo` in `machine.toml`. Nothing is rewritten.

> **Order matters: update `machine.toml` before running `apply`.** strata reads a missing repo folder as a repo with no files, so every managed file shows as `removed`, and apply would delete them from `$HOME`. (If you edited any of them, apply refuses instead.) `strata doctor` catches this: it reports a missing repo folder as an error.

## Commands

| Command | What it does |
|---|---|
| `strata` | Open the read-only [TUI](#the-tui) |
| `strata status` | List files that need attention; exit status 1 if any do |
| `strata doctor` | Check this machine's setup and list every problem, each with a fix; exit status 1 on errors |
| `strata diff` | Diff what's in `$HOME` against what apply would write |
| `strata apply` | Write changes into `$HOME`, then run hooks (`-n` to preview, `--force` to overwrite local changes) |
| `strata edit <file>` | Open the winning layer's source in your editor, show the diff, offer to apply |
| `strata add <file>` | Copy a file from `$HOME` into the repo — adopt a new file, or keep local edits (`--layer` to choose where) |
| `strata rm <file>` | Delete a file from its winning layer, then apply |
| `strata sync` | `git pull --ff-only` in the repo, then apply |
| `strata init` | Set up this machine: clone or use a repo, choose role layers, first apply |
| `strata upgrade` | Replace strata with the latest signed release (`--check` to only look) |
| `strata uninstall` | Remove strata itself — not your dotfiles (`-n` to preview) |

Every command has full help with examples: `strata <command> --help`.

<details>
<summary><code>apply</code> — dry runs, partial failures, hooks, symlinks</summary>

- `--dry-run` / `-n` lists what apply would do file by file — `would write`, `would chmod`, `would remove`, `would hook`, or `blocked` with the reason — and writes nothing.
- If a write fails for an I/O reason partway (full disk, unwritable folder), the files already written stay recorded and their hooks queued, so the next apply picks up where it stopped.
- Hooks run in `$HOME` with no time limit (a first `brew bundle` can take an hour). A failed or interrupted hook stays pending — `status` shows it — and the next apply retries it.
- A `$HOME` dotfile that's a **symlink** is never silently replaced: apply refuses it like a drifted file, and `--force` swaps in a regular file.
- Running apply when everything is clean prints `nothing to do`; it's always safe to re-run.

</details>

<details>
<summary><code>add</code> — choosing the layer</summary>

- Without `--layer`, the file goes to the layer that currently wins for it, or `base` for a new file. If the repo can't be planned right now (say, an undefined `{{var}}`), add refuses rather than guess `base` — which could push a work-only file to every machine.
- `--layer` must be a folder in the repo or one of this machine's layers, so a typo is an error, not a new layer. If that layer isn't the one this machine gets the file from, the copy is saved but `$HOME` is left alone, and add tells you which layer wins.
- Adding a file on the `substitute` list captures the *expanded* values; add warns you to restore the `{{tokens}}` by hand.
- Paths can be `.zshrc`, `~/.zshrc`, or absolute; files outside your home folder are rejected.

</details>

<details>
<summary><code>rm</code> — what happens to the <code>$HOME</code> copy</summary>

If no other layer provides the file, apply deletes it from `$HOME` too. If an earlier layer still provides it (removing a `work/` override, say), that layer wins again and the `$HOME` copy is rewritten. If apply would refuse right now, `rm` refuses *before* deleting anything. Deleting a layer file by hand, or with `git rm` then `sync` elsewhere, works the same way.

</details>

<details>
<summary><code>upgrade</code> — what gets verified</summary>

Nothing is replaced until every check passes: the release is newer (never a downgrade), `checksums.txt` has a valid signature from the strata release key, the signed checksums list this platform's archive, the archive's SHA-256 matches, and the new binary runs and reports that version. If anything fails, your strata is left exactly as it was.

Only release builds upgrade themselves. Homebrew installs use `brew upgrade strata`; source builds update with `git pull && sh install.sh`. strata never touches the network unless you run `upgrade` or `get.sh`.

</details>

## Configuration

### `dots.toml`

Lives at the repo root. Every section is optional, and a repo without one is valid: files are copied as-is with default permissions.

```toml
# Only these files get {{var}} replaced — everything else is copied
# byte-for-byte, so shell ${VARS} and other tools' {{ }} are safe.
# (Top-level keys must come before any [section].)
substitute = [".gitconfig", ".Brewfile"]

# Paths in a layer that aren't dotfiles. Ignoring a file that was
# already applied just stops managing it; the $HOME copy stays.
ignore = [".claude/settings.json", "**/*.log"]

# Default values; machine.toml [vars] overrides them per machine.
[vars]
email = "personal@example.com"
name  = "Your Name"

# glob → octal mode (000–777). The longest matching pattern wins.
# Without a rule: 644, or 755 if the repo copy is executable.
[permissions]
".ssh/**" = "600"

# Run after apply writes the file: in $HOME, via sh -c (cmd /C on Windows).
[hooks]
".Brewfile" = "brew bundle --file=~/.Brewfile"
```

Details worth knowing:

- **Variables** look like `{{email}}` (or `{{ email }}`). An undefined variable fails the whole apply — strata never writes a half-substituted file.
- **Patterns** use `**` to cross folders. Two equally long `[permissions]` patterns that disagree are an error, never a coin flip. Folders strata creates are 700 for a private file, else 755; existing folders are never changed. (git only stores the exec bit, which is why `.ssh` needs a rule.)
- **Always ignored**, with no config: `**/.DS_Store`, `**/._*`, `**/.Spotlight-V100`, `**/Thumbs.db`, `**/desktop.ini`.
- **Typos are errors.** An unknown key (`[hook]` for `[hooks]`) or a malformed pattern fails loudly instead of silently doing nothing.

### State file

`~/.local/state/strata/state.json` records the SHA-256 of everything strata wrote, plus any hooks waiting for a retry. You never edit it. Deleting it is safe: files that match the repo are adopted again, and the rest read `unmanaged`.

## The TUI

Bare `strata` opens a full-screen viewer that never changes anything. Its three tabs show which layer each file comes from here and on mac/linux/windows, each file's status, and where every variable, hook and permission comes from. Press `enter` on a file for its full story and a diff.

Keys: `←` `→` or `1`–`3` switch tabs · `↑` `↓` move · `enter` details · `d` full diff · `esc` close · `q` quit.

## Security note

- **Releases are signed.** CI signs `checksums.txt` with a dedicated SSH key that only `v*` tag runs can use, and `strata upgrade` and `get.sh` verify it against the key built into strata (fingerprint `SHA256:nMQXiQxd18neATjd15cvS8DQ5ihMQXbrgBa6xLKedNY`). Every release also has GitHub build provenance: `gh attestation verify <file> --repo D1srupt3d/strata`.
- **Hooks run as shell commands,** like git hooks or a Makefile. Only use dotfiles repos you trust — any dotfiles repo can run code anyway, since it controls your `.zshrc`.
- strata reads and writes only `$HOME`, your repo, and its own config and state files — plus `/etc/os-release` to detect your Linux distro, and its own binary when you run `upgrade`.

## What strata doesn't do

Secrets encryption, templates, symlink mode, partial file merging, `run_once` scripts, and targets outside `$HOME` (like `/etc`) are deliberately left out to keep the model small. [ROADMAP.md](ROADMAP.md) has the reasoning and what's planned.

## Hacking on strata

```sh
go build -o strata .        # build
go test ./...               # unit + end-to-end tests, all in temp folders
go vet ./... && gofmt -l .  # lint and format check (CI also runs staticcheck and govulncheck)
```

Architecture notes and project conventions are in [CLAUDE.md](CLAUDE.md).

<details>
<summary>Environment variables for testing and sandboxes</summary>

| Variable | Overrides | Default |
|---|---|---|
| `STRATA_HOME` | Target home folder | your home folder |
| `STRATA_CONFIG` | Path to `machine.toml` | `~/.config/strata/machine.toml` |
| `STRATA_STATE` | Path to the state file | `~/.local/state/strata/state.json` |
| `STRATA_BIN` | The binary `uninstall` deletes | the running binary |
| `STRATA_RELEASE_API` | Release endpoint for `upgrade` and `get.sh` | GitHub's latest release |
| `VISUAL` / `EDITOR` | Editor for `strata edit` (may include arguments, like `code --wait`) | `vi` |

Point the first three at a temp folder to try anything without risk:

```sh
STRATA_HOME=/tmp/fakehome STRATA_CONFIG=/tmp/m.toml STRATA_STATE=/tmp/s.json strata apply
```

</details>
