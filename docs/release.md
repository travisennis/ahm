# Release Process

`ahm` publishes downloadable binaries through GitHub Releases. GitHub Packages
is not used for CLI binaries.

## Installers

Unix-like systems:

```bash
curl -fsSL https://raw.githubusercontent.com/travisennis/ahm/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/travisennis/ahm/main/scripts/install.ps1 | iex
```

Both installers default to the latest GitHub release, download the matching
archive for the host OS and CPU architecture, verify the archive against
`checksums.txt`, and install the `ahm` binary.

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/travisennis/ahm/main/scripts/install.sh | sh -s -- v0.3.0
```

```powershell
$env:AHM_VERSION = "v0.3.0"
irm https://raw.githubusercontent.com/travisennis/ahm/main/scripts/install.ps1 | iex
```

Override the install directory:

```bash
curl -fsSL https://raw.githubusercontent.com/travisennis/ahm/main/scripts/install.sh | BIN_DIR="$HOME/bin" sh
```

```powershell
$env:AHM_INSTALL_DIR = "$HOME\bin"
irm https://raw.githubusercontent.com/travisennis/ahm/main/scripts/install.ps1 | iex
```

## Creating A Release

Release prep requires `svu`, `git-cliff`, and the local GoReleaser toolchain
from `just install-tools`.

1. Make sure all intended changes are merged and the worktree is clean.
2. Run the release prep script:

```bash
scripts/prepare-release.sh
```

The script uses `svu` to pick the next SemVer tag, uses `git-cliff` to update
`CHANGELOG.md`, runs `just release-check`, and prints the exact commit and tag
commands.

To override the calculated version, pass a tag explicitly:

```bash
scripts/prepare-release.sh v0.3.0
```

3. Review the changelog diff.
4. Commit the changelog:

```bash
git add CHANGELOG.md
git commit -m "chore(release): prepare v0.3.0"
```

5. Create and push the tag:

```bash
git tag -a v0.3.0 -m "v0.3.0"
git push origin main
git push origin v0.3.0
```

The release workflow runs GoReleaser for tags matching `v*`. GoReleaser builds
Linux, macOS, and Windows archives for amd64 and arm64, publishes them to the
GitHub Release, and uploads `checksums.txt`.

## Version Rules

- Release tags use SemVer with a `v` prefix, for example `v0.3.0`.
- `svu` calculates the next tag from Conventional Commit history.
- `git-cliff` generates `CHANGELOG.md` from Conventional Commit history.
- Archive names use the bare version, for example
  `ahm_0.3.0_darwin_arm64.tar.gz`.
- `internal/version.Binary` is the binary release version injected by
  GoReleaser.
- `internal/templates.Version` is the embedded workflow template version and
  only changes when the embedded workflow templates change.
