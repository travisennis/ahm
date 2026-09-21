# ahm

`ahm` is the records CLI for repo-local workflow state. It manages two record
families: tasks under `.ahm/tasks/` and ADRs under `docs/adr/`. It creates and
advances those records, regenerates a deterministic Markdown index for each
family, and validates their integrity.

`ahm` does nothing else. It does not run coding agents, ship workflow
procedures, commit, push, or patch project source. Project guidance, including
`AGENTS.md` and the prose under `docs/`, belongs to the project that owns it.

## Status

Active development. The tool is a tasks-and-ADRs records CLI with no
prescribed workflow; [ADR 022](docs/adr/022-reduce-ahm-to-a-tasks-and-adrs-records-cli.md)
records that boundary and [`CHANGELOG.md`](CHANGELOG.md) records release
history.

## Quickstart

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/travisennis/ahm/master/scripts/install.ps1 | iex
```

```bash
ahm init
ahm status
ahm task create "Add release workflow" --priority P2 --effort M --labels type:task,area:ci
ahm task ready
ahm task show 001
ahm adr create "Records only" --status accepted
ahm prime
```

Useful global flags:

- `--root <path>`: target repository root. Defaults to the nearest git root or
  `.ahm/config.json` parent; `init` falls back to the current directory.
- `--json`: print structured JSON.
- `--plain`: print stable line-oriented output.
- `--text`: print human-friendly text (the default).
- `--dry-run`: preview write operations for commands that support it.
- `--force`: override strict acceptance when supported.

For the full command, flag, output, and task-file contract, start with
[`docs/cli.md`](docs/cli.md).

`ahm prime` prints a live repository briefing: it regenerates indexes and
reports validation findings and record counts.
`AGENTS.md` is project-owned: `ahm init` and `--force` never create, overwrite,
or remove it.

## Safety

`ahm` never commits, stages, pushes, opens pull requests, or patches project
source, and it makes no network requests. Every write is explicit and confined
to state `ahm` owns: task files under `.ahm/tasks/`, ADRs under `docs/adr/`,
`.ahm/config.json`, the managed `.ahm/.gitignore`, and the generated indexes.
Records are ordinary committed project files; generated task indexes are
local-only. Git is the only program `ahm` runs.

## Development

For the full local setup, command catalog, verification expectations, and
commit conventions, see [`CONTRIBUTING.md`](CONTRIBUTING.md). For the module
map and architectural invariants, see [`ARCHITECTURE.md`](ARCHITECTURE.md).
For the documentation map, see [`docs/README.md`](docs/README.md).
For release publishing and installer details, see
[`docs/release.md`](docs/release.md).

Install Go 1.26.3 plus the local verification tools:

```bash
just install-tools
```

Use `just ci` for the read-only check suite that CI runs:

```bash
just ci
```

Use `just fix` for the mutating cleanup pass:

```bash
just fix
```

Use `just update-deps` to update Go module dependencies and tidy the module:

```bash
just update-deps
```

This repository uses `prek` with a pre-commit-compatible config:

```bash
prek install
prek install --hook-type commit-msg
```

Commit messages must follow Conventional Commits, for example
`feat: add release workflow` or `fix: handle missing task metadata`.
