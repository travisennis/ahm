#!/bin/sh
# Skip if no .go files are staged.
if ! git diff --cached --name-only | grep -q '\.go$'; then
  exit 0
fi
just tidy-check
