#!/usr/bin/env bash
set -euo pipefail

# Runs Clarity's end-to-end CLI integration smoke test.
#
# This test creates an isolated store in a Go temp dir via `--dir`, so it does not touch ~/.clarity,
# and it tears down automatically when the test exits.

# In some sandboxed/locked-down environments, the default Go build cache location may not be writable.
# Keep caches repo-local so CI and local runs are reliable.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export GOCACHE="${GOCACHE:-"$ROOT_DIR/.tmp/go-build-cache"}"
export GOMODCACHE="${GOMODCACHE:-"$ROOT_DIR/.tmp/go-mod-cache"}"
mkdir -p "$GOCACHE" "$GOMODCACHE"

go test -tags=integration ./internal/cli -count=1
