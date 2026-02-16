#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

ts="$(date -u +%Y%m%d-%H%M%S)"
out="${1:-server-perf-kit-${ts}.tar.gz}"

tar -czf "$out" \
  tools/server-perf-kit

echo "wrote package: $out"
