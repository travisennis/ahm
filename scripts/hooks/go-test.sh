#!/bin/sh
# Skip when the staged changes cannot affect the Go tests. The CLI reference
# inventories are compared against the command tree by a Go test, so those
# pages are test-relevant even though they are not Go sources.
if ! git diff --cached --name-only | grep -qE '\.go$|^docs/references/cli/'; then
  exit 0
fi
just test
