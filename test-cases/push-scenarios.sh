#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
exec ./test-cases/run.sh -run 'TestReceiptScenarios/(not-pushed|pushed-later)'
