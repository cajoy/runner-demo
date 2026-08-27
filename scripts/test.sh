#!/bin/sh
set -eu

test "$(sed -n '1p' hello.txt)" = 'hello from one Runner workflow'
printf '%s\n' 'test: hello.txt passed'

