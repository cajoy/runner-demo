#!/usr/bin/env bash
# Run the local half of the demo: execute lint, unit, and build on this
# machine, then sign and attach their portable receipt to HEAD.
set -euo pipefail

project_root=$(cd "$(dirname "$0")/.." && pwd -P)
cd "$project_root"

runner_version=$(jq -r '.runner.version' .local-ci/toolchain.lock)
case "$(uname -s):$(uname -m)" in
  Darwin:arm64) host_asset="runner_${runner_version}_darwin_arm64" ;;
  Linux:x86_64 | Linux:amd64) host_asset="runner_${runner_version}_linux_amd64" ;;
  Linux:aarch64 | Linux:arm64) host_asset="runner_${runner_version}_linux_arm64" ;;
  *) echo "demo_platform_unsupported" >&2; exit 2 ;;
esac
runner_binary=${RUNNER_BIN:-$project_root/.runner-bundle/$host_asset}

if [[ -n $(git status --porcelain=v1 --untracked-files=normal) ]]; then
  echo "demo_source_dirty: commit before signing a portable receipt" >&2
  exit 2
fi

echo "==> running api:preflight"
report=$("$runner_binary" run --project "$project_root" --json api:preflight)
printf '%s\n' "$report" | jq '{run_id, state, receipt_path}'
run_id=$(printf '%s' "$report" | jq -r '.run_id')

echo "==> signing and attaching the portable receipt to HEAD"
"$runner_binary" receipts attach \
  --project "$project_root" \
  --run "$run_id" \
  --commit HEAD \
  --signer local-alex \
  --json | jq '{commit, signer, receipt_digest, index_entries}'

echo
echo "Publish the branch and its receipt notes together:"
echo "  git push --atomic origin HEAD refs/notes/runner-receipts"
