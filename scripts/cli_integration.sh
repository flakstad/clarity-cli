#!/usr/bin/env bash
set -euo pipefail

# Runs Clarity's end-to-end CLI integration smoke test.
#
# This test creates an isolated store in a Go temp dir via `--dir`, so it does not touch ~/.clarity,
# and it tears down automatically when the test exits.

go test -tags=integration ./internal/cli -count=1
