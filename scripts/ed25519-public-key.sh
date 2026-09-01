#!/usr/bin/env bash
# Read a base64 Ed25519 seed on stdin; print its base64 public key.
# The seed never reaches the filesystem or the process table.
set -euo pipefail

seed_base64=$(cat)
work=$(mktemp -d "${TMPDIR:-/tmp}/runner-demo-pubkey.XXXXXX")
trap 'find "$work" -depth -delete' EXIT
umask 077

{
  printf '302e020100300506032b657004220420' | xxd -r -p
  printf '%s' "$seed_base64" | base64 -d
} >"$work/private.der"

openssl pkey -inform DER -in "$work/private.der" -pubout -outform DER -out "$work/public.der"
tail -c 32 "$work/public.der" | base64 | tr -d '\r\n'
echo
