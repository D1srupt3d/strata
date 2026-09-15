#!/bin/sh
# Regenerates the SSH-signature test fixtures using the REAL ssh-keygen, so
# strata's verifier is tested against the tool that makes release
# signatures — not against its own encoder. The throwaway private key lives
# only in a temp dir and is deleted on exit; only its public half and the
# signatures are kept. Run from anywhere: sh internal/release/testdata/gen.sh
set -eu
cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

ssh-keygen -q -t ed25519 -N '' -C strata-test -f "$tmp/key"
cp "$tmp/key.pub" test_key.pub

printf 'hello strata\n' > message.txt

# sha512 is ssh-keygen's default hash; sha256 is the other one it can emit.
cp message.txt "$tmp/msg"
ssh-keygen -q -Y sign -n strata-release -f "$tmp/key" "$tmp/msg"
mv "$tmp/msg.sig" message.sha512.sig

ssh-keygen -q -Y sign -n strata-release -O hashalg=sha256 -f "$tmp/key" "$tmp/msg"
mv "$tmp/msg.sig" message.sha256.sig

# Same key and message, different namespace: must NOT pass as a release sig.
ssh-keygen -q -Y sign -n some-other-purpose -f "$tmp/key" "$tmp/msg"
mv "$tmp/msg.sig" message.wrong-namespace.sig

echo "regenerated fixtures in $(pwd)"
