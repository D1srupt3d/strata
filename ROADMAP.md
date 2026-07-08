# roadmap

Versions are CalVer, `YYYY.0M.PATCH`, so 2026.07.0 is "shipped July 2026" and
2026.07.1 is "shipped again that same month". No promises on dates. This is a
personal tool that I use every day, which means things get built when they
annoy me enough.

Some context: I built strata because chezmoi's workflow drove me up a wall.
`dot_zshrc` filenames, `.tmpl` everywhere, Go template syntax for what was
usually a one-line difference between my work and home machines, and about
twenty commands where three would do. Strata is the tool I wished I had:
one repo that looks like my home directory, layers that stack (base, then OS,
then work/home role), and it never overwrites something I edited by hand.
Everything on this roadmap gets judged against that: does it keep the daily
workflow at three commands, or is it creeping back toward chezmoi?

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
- migrated my real dotfiles off chezmoi with byte-identical verification,
  then uninstalled chezmoi. It's not an experiment anymore, it's the thing
  managing my shell config right now.

## next — 2026.07.1: deleting a file shouldn't be a trap

Right now if I delete a file from the repo, strata just forgets about it and
the copy in `$HOME` lives on forever, unmanaged. That's the last piece of
"state I have to remember in my head", which is exactly what this tool exists
to kill.

Plan, roughly:

- state already knows every file strata has written. If a tracked file
  disappears from all layers, `status` shows it as `removed` and `apply`
  deletes it from `$HOME`
- same safety rules as overwrites: if I edited the file after the last
  apply, refuse and make me choose. Deleting drifted files silently would
  be worse than the current behavior
- probably a `strata rm .tmux.conf` convenience that deletes from the
  winning layer and applies in one step
- open question: what happens when a file leaves the work layer but still
  exists in base? Answer should fall out of the model (base wins again,
  file gets rewritten) but needs a test to prove it

## soon-ish

**CI and real releases.** Tests only run when I remember to run them. A
GitHub Actions workflow (test + vet + build for all three OSes) plus tagged
releases with prebuilt binaries would mean setting up a new machine doesn't
require a Go toolchain — download binary, `strata init <repo url>`, done.
That's the whole pitch of a dotfiles manager, so this should probably happen
before I buy my next laptop and not after.

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
- **directory permissions** — chezmoi made `~/.gnupg` itself 700; strata only
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

- **a template language.** The moment strata has `{{ if }}`, I've rebuilt
  chezmoi and should apologize. Layers cover whole-file differences, vars
  cover one-line differences, and I have not hit a third case in real use.
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
