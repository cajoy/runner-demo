#!/bin/sh
set -eu

case "${1:-}" in
  fail|success) ;;
  *)
    printf '%s\n' 'usage: ./scripts/set-result.sh fail|success' >&2
    exit 2
    ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
project_root=$(CDPATH= cd -- "$script_dir/.." && pwd -P)
state_dir="$project_root/.local-ci/state/demo"

umask 077
mkdir -p "$state_dir"
printf '%s\n' "$1" > "$state_dir/result"
printf 'demo final result: %s\n' "$1"

