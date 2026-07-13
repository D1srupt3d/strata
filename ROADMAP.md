# roadmap

Versions are CalVer, `YYYY.0M.PATCH`, so 2026.07.0 is "shipped July 2026" and
2026.07.1 is "shipped again that same month". No promises on dates. This is a
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

## soon-ish

**CI and real releases.** Tests only run when I remember to run them. A
GitHub Actions workflow (test + vet + build for all three OSes) plus tagged
releases with prebuilt binaries would mean setting up a new machine doesn't
require a Go toolchain — download binary, `strata init <repo url>`, done.
That's the whole pitch of a dotfiles manager, so this should probably happen
before I buy my next laptop and not after. GoReleaser gets me most of this
for one config file — binaries, checksums, changelogs, and a Homebrew tap.

**`strata upgrade` (self-updater).** Once releases exist: check the latest
tag, download the right binary, verify it against checksums.txt (never
skipping that — it's replacing an executable), and atomically swap it over
os.Executable(), same temp-file-and-rename trick apply already uses. Two
rules I've already decided: if the binary was installed by Homebrew, refuse
and say `brew upgrade strata` instead of fighting brew's bookkeeping; and
no silent background auto-update, ever — a tool that rewrites my shell
config updates when I tell it to. Maybe a one-line "update available"
notice in the TUI (cached, checked at most daily) so I actually find out.

**Bootstrap the personal Mac for real.** `strata init` from a git URL has
only ever run against test fixtures. The first real second-machine setup
will surface something dumb, it always does. I want that pain while the
code is fresh in my head.

**Run it on actual Linux.** The arch/distro layer detection is unit tested
but has literally never executed on a Linux box. I have a homelab; there's
no excuse.

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
- **directory permissions** — my old setup made `~/.gnupg` itself 700; strata only
  does files. Haven't hit a real problem from this yet but it's a known gap.
- **hook globs and run-once** — `".config/nvim/**" = "restart nvim somehow"`,
  and a way to run machine-setup scripts exactly once per machine instead of
  on every change. The second one smells like scope creep; sitting on it.
- **TUI write actions** — apply/add from inside the TUI. I made it read-only
  on purpose (a viewer you can trust completely is worth a lot), so if this
  happens it'll be opt-in and obvious, not default.
- **`strata doctor`** — checks your setup and says what's wrong: repo missing,
  machine.toml stale, state file referencing files that don't exist, etc.

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
