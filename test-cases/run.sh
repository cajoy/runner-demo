#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
exec go test -count=1 -timeout 30m -v "$@" ./test-cases -args -runner "${RUNNER_BIN:-$(command -v runner)}"
