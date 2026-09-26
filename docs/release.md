# Release Process

`ahm` publishes downloadable binaries through GitHub Releases. GitHub Packages
is not used for CLI binaries.

## Installers

Unix-like systems:

```bash
curl -fsSL https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.ps1 | iex
```

Both installers default to the latest GitHub release, download the matching
archive for the host OS and CPU architecture, verify the archive against
`checksums.txt`, and install the `ahm` binary.

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.sh | sh -s -- v0.3.0
```

```powershell
$env:AHM_VERSION = "v0.3.0"
irm https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.ps1 | iex
```

Override the install directory:

```bash
curl -fsSL https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.sh | BIN_DIR="$HOME/bin" sh
```

```powershell
$env:AHM_INSTALL_DIR = "$HOME\bin"
irm https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.ps1 | iex
```

## Creating A Release

Release prep requires:

- `svu`
- `git-cliff`
- GoReleaser, installed by `just install-tools`

Releases are prepared on `master`: the changelog commit lands there like any
other commit, and the tag is created and pushed from the same commit. CI runs
on the push, so confirm it is green before tagging.

## Release Notes

`CHANGELOG.md` is generated from Conventional Commits, so it cannot carry
release-specific prose: `just prepare-release` overwrites the whole file. Write
the release body in `docs/releases/<tag>.md` instead, where the filename
matches the tag exactly, including the `v` prefix.

The release workflow publishes `docs/releases/<tag>.md` as the GitHub Release
body when that file exists on the tagged commit, and falls back to the
changelog GoReleaser generates from the commit log when it does not. A release
that removes a command, a configuration key, or a record family must have a
notes file: those are the only consumer-facing details the generated changelog
cannot state, and the removed surfaces need to be named rather than left for a
reader to infer from commit subjects.

A release notes file is committed with the changelog, before the tag is pushed,
so the tagged commit already contains it. Keep it self-contained — it is
rendered on a release page, not in the docs tree, so link to documentation
with absolute URLs.

## Weekly Release Checklist

1. Make sure all intended changes are merged and the worktree is clean.
2. Update local refs and inspect the current release baseline:

```bash
git fetch --tags origin
git status --short
git tag --list 'v*' --sort=-v:refname | head
svu current --tag.output tag.prefix
svu next --tag.output tag.prefix
```

1. Run the release prep script:

```bash
just prepare-release
```

The script uses `svu` to pick the next SemVer tag, uses `git-cliff` to update
`CHANGELOG.md`, runs `just release-check`, and prints the exact commit and tag
commands.

To override the calculated version, pass a tag explicitly:

```bash
scripts/prepare-release.sh v0.3.0
```

1. Review the changelog diff.
2. Commit the changelog on `master` and push it:

```bash
git add CHANGELOG.md
git commit -m "chore(release): prepare v0.3.0"
git push origin master
gh run list --limit 3
```

1. Once CI is green on that commit, create and push the tag:

```bash
git tag -a v0.3.0 -m "v0.3.0"
git push origin v0.3.0
```

The release workflow runs GoReleaser for tags matching `v*`. GoReleaser builds
Linux, macOS, and Windows archives for amd64 and arm64, publishes them to the
GitHub Release, and uploads `checksums.txt`. It publishes
`docs/releases/<tag>.md` as the release body when that file exists.

1. Watch the release workflow:

```bash
gh run list --workflow Release --limit 5
gh run watch <run-id> --exit-status
```

1. Verify the published release:

```bash
gh release view v0.3.0 --json tagName,name,url,assets
```

Check that the release contains:

- `ahm_<version>_darwin_amd64.tar.gz`
- `ahm_<version>_darwin_arm64.tar.gz`
- `ahm_<version>_linux_amd64.tar.gz`
- `ahm_<version>_linux_arm64.tar.gz`
- `ahm_<version>_windows_amd64.zip`
- `ahm_<version>_windows_arm64.zip`
- `checksums.txt`

1. Smoke-test the installer:

```bash
tmp="$(mktemp -d)"
BIN_DIR="$tmp/bin" sh scripts/install.sh v0.3.0
"$tmp/bin/ahm" --version
```

The version command must print the bare SemVer version, for example `0.3.0`.

1. Check the final local state:

```bash
git status --short
git tag --points-at HEAD
```

## Version Rules

- Release tags use SemVer with a `v` prefix, for example `v0.3.0`.
- A release notes file is named for the tag it belongs to, for example
  `docs/releases/v0.3.0.md`, and is committed before the tag is pushed.
- `svu` calculates the next tag from Conventional Commit history.
- `git-cliff` generates `CHANGELOG.md` from Conventional Commit history.
- Archive names use the bare version, for example
  `ahm_0.3.0_darwin_arm64.tar.gz`.
- `internal/version.Binary` is the binary release version injected by
  GoReleaser.
