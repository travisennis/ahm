# ahm

`ahm` is an agent harness manager. It installs and manages a repo-local
`.agents` workflow for agent tasks, research notes, ExecPlans, and generated
indexes.

`ahm` replaces the earlier `agent-workflow-scaffold` skill as the owner of this
workflow. The canonical workflow templates live in this repository and are
embedded into the CLI at build time.

## Status

Initial implementation. The CLI supports workflow install/upgrade/status,
native task index generation, task management, and handing a resolved task to a
supported external coding-agent CLI.

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
ahm task work 001
```

Useful global flags:

- `--root <path>`: target repository root. Defaults to the nearest git root or
  current directory.
- `--json`: print structured JSON.
- `--plain`: print stable line-oriented output.
- `--dry-run`: preview write operations for commands that support it.
- `--force`: remove conflicting legacy instruction files or override strict
  acceptance when supported.

For the full command, flag, output, and task-file contract, start with
[`docs/cli.md`](docs/cli.md).

`ahm context` gives a live repository briefing; scoped
`ahm context task|plan|adr|research|docs` prints managed-work references for
ahm-managed artifacts, while project `AGENTS.md` owns workflow routing.
`AGENTS.md` is project-owned: `ahm init`, `ahm upgrade`, and `--force` never
create, overwrite, or remove it.

## Safety

`ahm` does not run git commits, pushes, PRs, or source-code patches itself.
Write commands are explicit and operate on the `.agents` workflow files unless a
future command states otherwise. `ahm task work <id>` is an explicit delegation
command: it validates the task workflow state, then invokes the selected
external coding-agent CLI from the repository root. Review and commit run by
default (`--no-review` / `--no-commit` to opt out). The delegated agent and
project hooks own the actual git operation.

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

Commit messages and pull request titles must follow Conventional Commits, for
example `feat: add release workflow` or `fix: handle missing task metadata`.
