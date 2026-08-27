#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
project_root=$(CDPATH= cd -- "$script_dir/.." && pwd -P)
result_file="$project_root/.local-ci/state/demo/result"
result=fail

if test -f "$result_file"; then
  result=$(sed -n '1p' "$result_file")
fi

if test "$result" = success; then
  printf '%s\n' 'finish: configured success'
  exit 0
fi

printf '%s\n' 'finish: intentional demo failure; set result to success and resume this run' >&2
exit 42

