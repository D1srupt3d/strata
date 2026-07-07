#!/bin/sh
# strata installer: builds the binary, copies it to ~/.local/bin, and makes
# sure that directory is on your PATH (appends one line to your shell rc,
# only if it isn't there already). Safe to re-run any time.
#
# Override the install dir with:  STRATA_BIN_DIR=/somewhere sh install.sh
set -eu

BIN_DIR="${STRATA_BIN_DIR:-$HOME/.local/bin}"
cd "$(dirname "$0")"

echo "building strata..."
go build -o strata .

mkdir -p "$BIN_DIR"
cp strata "$BIN_DIR/strata"
chmod 755 "$BIN_DIR/strata"
echo "installed $BIN_DIR/strata"

# Already reachable? Then we're done.
case ":$PATH:" in
*":$BIN_DIR:"*)
    echo "$BIN_DIR is already on your PATH."
    echo "try it: strata --help"
    exit 0
    ;;
esac

# Pick the rc file for the user's login shell.
rc="$HOME/.profile"
case "${SHELL:-}" in
*/zsh) rc="$HOME/.zshrc" ;;
*/bash) rc="$HOME/.bashrc" ;;
esac

line="export PATH=\"$BIN_DIR:\$PATH\""
if [ -f "$rc" ] && grep -Fq "$line" "$rc"; then
    echo "PATH line already present in $rc (restart your terminal to pick it up)"
else
    printf '\n# added by strata install.sh\n%s\n' "$line" >>"$rc"
    echo "added $BIN_DIR to PATH in $rc"
fi
echo "open a new terminal (or run: source $rc), then try: strata --help"
