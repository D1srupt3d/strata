#!/bin/sh
# strata installer for new machines: downloads the latest release, verifies
# its signature and checksum, and installs the binary — no git clone, no Go.
#
#   curl -fsSL https://raw.githubusercontent.com/D1srupt3d/strata/main/get.sh | sh
#
# Afterwards, update with `strata upgrade`. Re-running this reinstalls the
# latest release (it's also the recovery path if the release key changes).
# Installs to ~/.local/bin; override with STRATA_BIN_DIR=/somewhere. macOS and
# Linux only — on Windows, download the .zip from the releases page.
set -eu

API="${STRATA_RELEASE_API:-https://api.github.com/repos/D1srupt3d/strata/releases/latest}"
BIN_DIR="${STRATA_BIN_DIR:-$HOME/.local/bin}"
# The strata release-signing public key — the same key strata embeds in
# internal/release/release_key.pub (a test keeps the two identical).
TRUSTED_KEY="${STRATA_GET_TRUSTED_KEY:-ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJHxsY5942gsckPie8B1lzG5HYmhqZhnpj+FqQ7k2grQ}"
NAMESPACE=strata-release
RELEASES=https://github.com/D1srupt3d/strata/releases

die() {
    echo "strata install: $*" >&2
    exit 1
}
for tool in curl tar ssh-keygen uname sed awk grep tr mktemp; do
    command -v "$tool" >/dev/null 2>&1 || die "needs '$tool', which isn't installed"
done
if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
    die "needs sha256sum or shasum to check the download"
fi

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) die "unsupported OS $(uname -s) — download a release from $RELEASES" ;;
esac
case "$(uname -m)" in
arm64 | aarch64) arch=arm64 ;;
x86_64 | amd64) arch=amd64 ;;
*) die "unsupported CPU $(uname -m) — download a release from $RELEASES" ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -H 'Accept: application/vnd.github+json' "$API" -o "$tmp/release.json" ||
    die "couldn't reach $API"
# One JSON field per line, whether or not the response is pretty-printed.
tr ',' '\n' <"$tmp/release.json" >"$tmp/fields"
tag=$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "$tmp/fields" | head -n 1)
[ -n "$tag" ] || die "no published release found at $API"
version=${tag#v}
archive="strata_${version}_${os}_${arch}.tar.gz"

asset_url() {
    sed -n "s#.*\"browser_download_url\": *\"\\([^\"]*/$1\\)\".*#\\1#p" "$tmp/fields" | head -n 1
}
for f in "$archive" checksums.txt checksums.txt.sig; do
    url=$(asset_url "$f")
    [ -n "$url" ] || die "release $tag has no $f — nothing installed (releases from before signing can't be installed this way)"
    curl -fsSL "$url" -o "$tmp/$f" || die "download failed: $f"
done

# 1. The checksum list must be signed by the strata release key.
printf 'strata-release namespaces="%s" %s\n' "$NAMESPACE" "$TRUSTED_KEY" >"$tmp/allowed_signers"
ssh-keygen -Y verify -f "$tmp/allowed_signers" -I strata-release -n "$NAMESPACE" \
    -s "$tmp/checksums.txt.sig" <"$tmp/checksums.txt" >/dev/null 2>&1 ||
    die "signature check FAILED for release $tag — nothing installed"

# 2. The archive must be exactly the one that was signed.
want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
have=$(sha256 "$tmp/$archive")
{ [ -n "$want" ] && [ "$want" = "$have" ]; } || die "checksum mismatch for $archive — nothing installed"

# 3. Take only the binary, and make sure it runs.
mkdir "$tmp/x"
tar -xzf "$tmp/$archive" -C "$tmp/x" strata 2>/dev/null || die "$archive has no strata binary — nothing installed"
chmod 755 "$tmp/x/strata"
"$tmp/x/strata" --version >/dev/null 2>&1 || die "the downloaded strata won't run — nothing installed"

# 4. Install atomically (copy next to the target, then rename over it).
mkdir -p "$BIN_DIR"
if [ -e "$BIN_DIR/strata" ]; then
    echo "replacing $("$BIN_DIR/strata" --version 2>/dev/null || echo 'the existing strata') with release $version"
fi
cp "$tmp/x/strata" "$BIN_DIR/.strata-new.$$"
mv -f "$BIN_DIR/.strata-new.$$" "$BIN_DIR/strata"
echo "installed $BIN_DIR/strata ($("$BIN_DIR/strata" --version))"

# PATH: same rules as install.sh — a login profile, never a managed rc file.
case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*)
    rc="$HOME/.profile"
    case "${SHELL:-}" in
    */zsh) rc="$HOME/.zprofile" ;;
    */bash) [ -f "$HOME/.bash_profile" ] && rc="$HOME/.bash_profile" ;;
    esac
    line="export PATH=\"$BIN_DIR:\$PATH\""
    if ! { [ -f "$rc" ] && grep -Fq "$line" "$rc"; }; then
        printf '\n# added by strata install.sh\n%s\n' "$line" >>"$rc"
        echo "added $BIN_DIR to PATH in $rc (open a new terminal to pick it up)"
    fi
    ;;
esac

found=$(command -v strata 2>/dev/null || true)
if [ -n "$found" ] && [ "$found" != "$BIN_DIR/strata" ]; then
    echo "note: $found comes first on your PATH — that's the one 'strata' runs"
fi
echo "to update later: strata upgrade"
