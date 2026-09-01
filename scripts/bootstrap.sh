#!/usr/bin/env bash
# Prepare this checkout for the portable-receipt demo:
# install the pinned Runner toolchain, ensure both demo signers exist, and
# render the protected receipt policy from their public keys.
set -euo pipefail

project_root=$(cd "$(dirname "$0")/.." && pwd -P)
cd "$project_root"

runner_version=$(jq -r '.runner.version' .local-ci/toolchain.lock)
bundle="$project_root/.runner-bundle"
keychain_service=cajoy.runner.receipt-signing
local_signer=local-alex
ci_signer=github

case "$(uname -s):$(uname -m)" in
  Darwin:arm64) host_asset="runner_${runner_version}_darwin_arm64" ;;
  Linux:x86_64 | Linux:amd64) host_asset="runner_${runner_version}_linux_amd64" ;;
  Linux:aarch64 | Linux:arm64) host_asset="runner_${runner_version}_linux_arm64" ;;
  *) echo "demo_platform_unsupported" >&2; exit 2 ;;
esac

if [[ ! -x "$bundle/$host_asset" ]]; then
  echo "==> downloading Runner $runner_version"
  mkdir -p "$bundle"
  gh release download "$runner_version" --repo cajoy/runner --dir "$bundle" --clobber
  (cd "$bundle" && shasum -a 256 -c checksums.txt >/dev/null)
  chmod 755 "$bundle/$host_asset"
fi

runner_binary=${RUNNER_BIN:-$bundle/$host_asset}

echo "==> installing the committed toolchain"
"$runner_binary" toolchain install \
  --project "$project_root" \
  --source-dir "$bundle" \
  --json

# Ed25519 signer keys live in the login Keychain. The local signer proves
# developer runs; the CI signer's private half becomes a GitHub Actions secret.
ensure_signer() {
  local account=$1
  if security find-generic-password -a "$account" -s "$keychain_service" -w >/dev/null 2>&1; then
    return 0
  fi
  echo "==> generating Ed25519 signer $account" >&2
  local work
  work=$(mktemp -d "${TMPDIR:-/tmp}/runner-demo-signer.XXXXXX")
  openssl genpkey -algorithm ED25519 -out "$work/private.pem" >/dev/null 2>&1
  openssl pkey -in "$work/private.pem" -outform DER -out "$work/private.der"
  local seed
  seed=$(tail -c 32 "$work/private.der" | base64 | tr -d '\r\n')
  security add-generic-password -U -a "$account" -s "$keychain_service" -w "$seed"
  find "$work" -depth -delete
}

public_key() {
  security find-generic-password -a "$1" -s "$keychain_service" -w |
    "$project_root/scripts/ed25519-public-key.sh"
}

ensure_signer "$local_signer"
ensure_signer "$ci_signer"

origin=$(git remote get-url origin)
# Runner derives one credential-free repository identity from the origin URL;
# SSH and HTTPS clones of the same GitHub repository share it.
origin_identity=$(printf '%s' "$origin" |
  sed -E 's#^[a-z+]+://##; s#^[^@/]+@##; s#^([^/:]+):#\1/#; s#\.git$##' |
  tr 'A-Z' 'a-z')
repository_id="sha256:$(printf 'runner.repository.v1\0%s' "$origin_identity" |
  shasum -a 256 | awk '{print $1}')"

echo "==> rendering .runner/receipt-policy.yaml"
sed \
  -e "s|@LOCAL_PUBLIC_KEY@|$(public_key "$local_signer")|" \
  -e "s|@CI_PUBLIC_KEY@|$(public_key "$ci_signer")|" \
  -e "s|@REPOSITORY_ID@|$repository_id|" \
  .runner/receipt-policy.yaml.template >.runner/receipt-policy.yaml

echo
echo "repository_id=$repository_id"
echo "local signer:  $local_signer"
echo "ci signer:     $ci_signer"
echo
echo "Publish the CI signing key once, from a trusted shell:"
echo "  security find-generic-password -a $ci_signer -s $keychain_service -w |"
echo "    gh secret set RUNNER_RECEIPT_SIGNING_KEY --repo cajoy/runner-demo"
