# roadmap

Versions are CalVer, `YYYY.M.PATCH` (un-padded month — GoReleaser enforces
semver and rejects `2026.08.0`), so 2026.8.0 is "shipped August 2026" and
2026.8.1 is "shipped again that same month". No promises on dates. This is a
personal tool that I use every day, which means things get built when they
annoy me enough.

Some context: I moved here from another dotfiles manager — great, mature software that
just never fit how I think. I wanted real filenames instead of `dot_zshrc`,
layers instead of templates for what was usually a one-line difference
between my work and home machines, and a command set small enough to hold in
my head. Strata is the tool shaped like my brain: one repo that looks like my
home directory, layers that stack (base, then OS, then work/home role), and
it never overwrites something I edited by hand. Everything on this roadmap
gets judged against that: does it keep the daily workflow at three commands,
or is it creeping back toward the complexity I moved away from?

## done — 2026.07.0

The whole core, honestly:

- layer engine with whole-file-wins resolution (base → os → distro → role)
- three-way drift detection so `apply` knows the difference between "repo
  changed", "I edited the file", and "both" — and refuses to clobber my edits
- `{{var}}` substitution, but only for files that opt in, because dotfiles
  are full of `${VAR}` and `{{ }}` that must never be touched
- permission globs (git doesn't store modes, ssh cares deeply)
- hooks that run only when the file they watch actually changed
- the seven commands: init / apply / diff / status / edit / add / sync
- a read-only TUI when you run bare `strata` — layers, files-per-OS matrix,
  var provenance, per-file drilldown
- migrated my real dotfiles over with byte-identical verification,
  then retired the old manager. It's not an experiment anymore, it's the thing
  managing my shell config right now.

## done — 2026.07.1: deleting a file isn't a trap anymore

Used to be that deleting a file from the repo just orphaned the copy in
`$HOME` forever. Now: if a file strata has written disappears from every
layer, `status` shows it as `removed` and `apply` deletes it — with the
same safety rules as overwrites (edited the file since last apply? it
refuses and makes me choose). Stale state entries clean themselves up,
and `strata rm <file>` deletes from the winning layer and applies in one
step. The open question from the plan — file leaves the work layer but
still exists in base — resolved exactly how the model predicts (base wins
again, file gets rewritten), and there's a test pinning that down.

## done — 2026.8.0: CI and real releases

GitHub Actions runs gofmt + vet + build + test on macOS, Linux, and Windows
for every push, and pushing a tag has GoReleaser build all six OS/arch
binaries with checksums and a changelog. The release workflow re-runs the
tests before publishing, so a tagged commit with a red suite can't ship.
(I tried auto-releasing on a `release:` commit prefix for about an hour and
killed it — too much magic. Tags are the release decision.)

Immediately worth it: the first-ever Windows run caught two test bugs (no
POSIX file modes there, and a fake home of `/Users/x` isn't absolute without
a drive letter), and the first-ever Linux run just passed. v2026.8.0 is live
with downloadable binaries. Still open from the original wishlist: the
Homebrew tap, which needs its own repo and token.

## done — 2026.9.0: the edges caught up with the core

The first real machine setup (my work Mac) surfaced something dumb, exactly
as predicted below — several somethings, actually. So I sat down for a proper
review of the whole thing, and the verdict was fair: a careful engine with
soft edges. The core was fine; the wiring around it wasn't. Every fix went in
test-first, and the command layer went from half-tested to about three
quarters:

- a `[permissions]` rule added later now actually reaches files that were
  already applied — new `chmod` status. It never loosens a mode I tightened
  by hand, and never fires on Windows.
- a failed hook isn't forgotten anymore: it stays pending in state.json, shows
  up in `status`, and the next apply retries it until it works. Hooks also run
  in `$HOME` now, so relative paths mean the same thing every time.
- `strata add` refuses to guess `base/` when it can't plan — it once put my
  work git identity in the layer every machine gets
- typos in `dots.toml`/`machine.toml` (`[hook]` for `[hooks]`) are errors
  instead of silent no-ops, and two equally specific permission rules that
  disagree are an error instead of a coin flip
- `status` exits 1 when something needs attention, so it's finally scriptable
- `--dry-run` tells the truth about blocked files and the all-or-nothing rule
- symlinked dotfiles don't get silently swapped for regular files; `--force`
  if I mean it
- `init` respects `--layers ""`, warns when the repo isn't a git clone (like
  a downloaded zip), and lists the vars still sitting on their defaults
- `edit` honors `$VISUAL` and `code --wait`-style editors
- state.json has a format version and a lock, and writes fsync before the
  rename
- `install.sh` stopped editing `.zshrc` — a managed dotfile, so the first
  apply would have flagged the installer's own line as drift. PATH goes in
  the login profile now.
- CI pins every action to a commit SHA and runs the race detector; releases
  ship signed build provenance

Still open: vars per layer (below) — the thing that actually bit me on the
work Mac.

## done — 2026.9.1: updating without a checkout

Updating strata used to mean `git pull` in the repo and `sh install.sh` — so
every machine needed the source and a Go toolchain. Now `strata upgrade`
replaces itself with the latest release, and a brand-new machine gets one
with a single `curl … get.sh | sh`. Neither installs anything it can't prove
is mine: release CI signs checksums.txt with a dedicated SSH key that lives
in a GitHub environment only tag builds can open, and strata checks that
signature — then "is this actually newer", then the checksum, then "does
the new binary even start" — before it swaps a single byte. Zero new
dependencies: the signature check is a few dozen lines of standard library.
I looked hard at Sigstore instead and walked away; it would have nearly
tripled the dependency tree of a tool whose whole appeal is being small.

The rules from the plan held up: no background update checks, ever;
Homebrew installs belong to Homebrew; and a binary I built from source is
mine to update, so `upgrade` leaves it alone. The first CI run after the
merge caught a very Windows bug — git rewrote the line endings of a signed
test fixture, so its signature stopped matching — which is exactly why the
Windows leg exists.

## done — September 2026: the first real runs

Two items that were never about writing code. The personal Mac went from
nothing to managed with one `get.sh` line and `strata init <git-url>` — the
first time init had cloned a real repo instead of a test fixture. And strata
ran on actual Linux in the homelab, so the distro detection from
`/etc/os-release` finally executed somewhere other than a unit test. I braced
for something dumb both times, the way the work Mac delivered in 2026.9.0.
Nothing showed up — which mostly says the fixture tests were testing the
right things.

## soon-ish

**Vars per layer.** `[layer_vars.work]` in dots.toml, so picking the work
layer at init brings the work values with it instead of me remembering to
hand-edit machine.toml. Precedence follows the file stack: defaults → OS
layer → role layer → machine.toml. The format's decided and the provenance
plumbing the TUI needs is already in.

**Homebrew tap.** The loose end from 2026.8.0. Now that releases are signed,
GoReleaser can publish a formula to a `homebrew-strata` tap on every tag,
and `strata upgrade` already knows to step aside for Homebrew installs.
Needs its own repo and a token that can write to only that repo.

**Scriptable output.** `strata status --json` (and probably `diff --json`) so
a shell prompt or a script can ask "anything drifted?" without parsing text
meant for humans. `status` already exits 1 when something needs attention;
this is the structured version of the same answer.

**Add whole directories.** `strata add ~/.config/nvim` should adopt every
file under it in one go instead of me running `add` file by file — using the
same ignore rules apply does, so editor scratch and caches don't sneak in.

**Rehearse a key rotation.** The release-signing key is a single point of
trust. The plan is written down (ship one release that trusts both keys,
then switch), but I'd rather do it once calmly than for the first time in a
hurry — and decide what "the key leaked" really looks like beyond
"reinstall with get.sh".

**Shell completions.** Cobra generates them for free (`strata completion
zsh`), I just haven't wired the install script to put them anywhere. Tab
completion for file arguments (`strata edit .zs<tab>`) would be nice too
and is not free — needs the completion function to run the resolver.

## someday, maybe

Stuff I'd take a weekend on if the itch hits, in rough order of likelihood:

- **conflict merge assist** — when repo and $HOME both changed, all you can
  do today is pick a side. A three-way merge (or just launching `$EDITOR`
  on a merged view) would be kinder. Needs the state file to store content,
  not just hashes, so it's not free.
- **directory permissions** — my old setup made `~/.gnupg` itself 700. Folders
  strata *creates* are now as private as the file that needed them (a 600
  file gets a 700 folder), but there's still no way to set or enforce a mode
  on a folder that already exists.
- **hook globs and run-once** — `".config/nvim/**" = "restart nvim somehow"`,
  and a way to run machine-setup scripts exactly once per machine instead of
  on every change. The second one smells like scope creep; sitting on it.
- **TUI write actions** — apply/add from inside the TUI. I made it read-only
  on purpose (a viewer you can trust completely is worth a lot), so if this
  happens it'll be opt-in and obvious, not default.
- **`strata doctor`** — checks your setup and says what's wrong: repo missing,
  machine.toml stale, state file referencing files that don't exist, etc.
- **Windows installer** — a `get.ps1` twin of get.sh, so Windows doesn't mean
  downloading a zip by hand. Waiting on me using strata on Windows for more
  than CI.
- **opt-in update notice** — a cached, at-most-daily "update available" line
  in the TUI. Off by default; the no-background-network rule stands.
- **`strata upgrade --version`** — deliberately install a specific older
  signed release to back out a bad one. Plain `upgrade` still never
  downgrades; this would be the explicit, eyes-open exception.

## not doing

Writing these down so future me doesn't relitigate them:

- **a template language.** Other managers do templates extremely well —
  if I ever want them back, I know where to find them. Layers cover
  whole-file differences, vars cover one-line differences, and I have not
  hit a third case in real use.
- **symlink mode.** Symlinks can't express "base plus work bits", Windows
  hates them, and half my tools misbehave with symlinked config.
- **partial-file merging.** "Later layer wins the whole file" is the reason
  I can predict what any machine gets by looking at the repo. Not trading
  that away for cleverness.
- **files outside `$HOME`.** /etc is a job for real config management.
- **wrapping git.** `sync` pulls before applying because that's a workflow
  step; everything else is just git in a normal repo, which I already know
  how to use.
- **built-in secrets encryption.** Keys live in 1Password, the repo holds
  public halves and pointers. If that ever changes it'll be via age + a
  documented pattern, not a homegrown crypto layer.
