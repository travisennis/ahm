# Testing

`just test` runs the package tests; [`CONTRIBUTING.md`](../../CONTRIBUTING.md)
owns the command catalog and the verification policy. This guide covers the
sandbox those tests run in, which is what keeps a mutating test from writing to
a developer's real workflow records.

## The Test Sandbox

A test may only resolve workflow paths inside the system temporary directory,
the directory `os.TempDir()` names and `t.TempDir()` creates under. Every path
`ahm` touches derives from one of two roots, and the test binary confines both:

- The **project root**, detected from the working directory or named by
  `--root`.
- The **store root**, `AHM_HOME` or `~/.ahm`.

`TestMain` in `internal/ahm/cli_integration_test.go` pins `AHM_HOME` to a
scratch directory whenever the inherited value is unset or outside the
temporary directory, so an in-process test that never chooses a store root
still resolves a scratch one, and the built integration binary inherits the
pin. Tests that are about `AHM_HOME` itself use
`runCLIFromDirKeepingEnv`.

The pin is the sandbox; the guard is the tripwire. `TestMain` also installs
`assertResolvedRootWithinTempDir` (`internal/ahm/test_helpers_test.go`) on
`resolvedRootHook`, which `resolveWorkflowPaths`
(`internal/ahm/workflow_paths.go`) calls with the project root it resolves a
layout for, and `storeRoot` (`internal/ahm/store.go`) calls with the store root
it reads from the environment or the home directory. The guard fires on both
roots rather than on each derived path, because every derived path — records,
indexes, the record lock, the store-state lock, the store's project directory —
comes from one of them.

A resolved root outside the sandbox panics with the offending path and the
temporary directory, and stops the run. A suite that continued past it would
keep writing through a root the test does not own, and reaching the developer's
real `~/.ahm` is the damage worth failing loudly for. Production leaves the
hook empty, so an `ahm` run pays one call per root resolution and nothing else.

`withinTempDir` canonicalizes both sides before comparing, because one
temporary directory has several spellings: macOS makes `/var` a symlink to
`/private/var`, and a Windows current directory can resolve through an 8.3
short name. `t.TempDir` and `os.Getwd` do not agree on which spelling they
return, so a literal comparison would call a temporary path external.

## Writing A Test That Stays Inside The Sandbox

- Build fixtures with `t.TempDir()`: `projectRoot`, `setupAhmRepo`,
  `newGitRepo`, and `writeMetadataFile` already do.
- Run commands through `runCLI` or `runCLIFromDir` rather than calling `Main`
  with the process working directory at the repository root, where root
  detection resolves the checkout.
- Take the store root from the pin in an ordinary test, or set `setStoreHome`
  when the test pre-populates a store, including when the test builds an `app`
  directly.
- Reach the default store root by moving the home directory the store derives
  it from — `t.Setenv("HOME", t.TempDir())` plus `USERPROFILE` for Windows —
  instead of reading the machine's own home.

## Related Docs

- [Safety and permissions](../guardrails/safety-and-permissions.md)
- [`CONTRIBUTING.md`](../../CONTRIBUTING.md)
- [`ARCHITECTURE.md`](../../ARCHITECTURE.md)
