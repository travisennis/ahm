#!/usr/bin/env bash
set -euo pipefail

branch="$(git branch --show-current 2>/dev/null || true)"
if [[ "$branch" == "master" ]]; then
  echo "error: direct commits to master are disabled; work on a feat/<slug> branch and merge via PR" >&2
  exit 1
fi
exit 0
