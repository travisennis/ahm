---
status: accepted
date: 2026-07-12
decision-makers: Travis Ennis
---
# Scrub inherited Git repository-location environment

## Context and Problem Statement

Define the subprocess boundary that prevents inherited Git repository-location variables from redirecting ahm-owned Git commands.

## Decision Drivers

- A Git hook may export repository-location variables that override an explicit
  `git -C <root>` target.
- Ahm commands and test fixtures must never redirect Git reads or writes into
  the invoking repository by inheriting hook state.
- Normal user Git configuration, executable lookup, and non-location
  environment should remain available to subprocesses.
- The boundary must behave consistently for production helpers and tests.

## Considered Options

- Inherit the complete environment and rely on `git -C`.
- Clear the complete subprocess environment and rebuild an allowlist.
- Inherit the environment except for Git repository-location variables, and
  also clear those variables once at test-process startup.

## Decision Outcome

Chosen option: inherit the environment except for `GIT_DIR`, `GIT_WORK_TREE`,
`GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY`, and `GIT_COMMON_DIR`, because these
variables can redirect repository metadata, the worktree, or the index despite
an explicit `git -C <root>`, while the remaining environment contains useful
and expected Git configuration.

All ahm-owned Git subprocess helpers set this filtered environment explicitly.
The Go test process additionally removes the same variables before fixtures
run, protecting direct fixture commands and subprocesses that are outside the
production helper path. Tests that deliberately set these variables verify
that the shared Git command boundary remains isolated.

### Consequences

- Good, because hook-injected repository state cannot redirect ahm-owned Git
  operations or test fixtures into the host repository.
- Good, because user identity, global configuration, PATH, locale, and other
  unrelated environment remain inherited.
- Bad, because callers cannot intentionally use these five environment
  variables to redirect ahm-owned Git commands; they must instead pass the
  intended repository root through ahm's explicit command boundary.

## More Information

- Task 183
- `internal/ahm/git.go`
- `internal/ahm/cli_integration_test.go`
- `internal/ahm/records_test.go`
