#!/usr/bin/env sh
set -eu

mkdir -p .local-ci-out
printf '%s\n' 'hello from GitHub Runner' >.local-ci-out/deploy-hello.txt
test "$(sed -n '1p' .local-ci-out/deploy-hello.txt)" = 'hello from GitHub Runner'

