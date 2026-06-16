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

```bash
ahm init
ahm status
ahm task create "Add release workflow" --priority P2 --effort M --labels type:task,area:ci
ahm task ready
ahm task show 001
ahm task work 001 --review --commit
```

Useful global flags:

- `--root <path>`: target repository root. Defaults to the nearest git root or
  current directory.
- `--json`: print structured JSON.
- `--plain`: print stable line-oriented output.
- `--dry-run`: preview write operations for commands that support it.
- `--force`: overwrite managed workflow files when supported.

For the full command, flag, output, and task-file contract, start with
[`docs/cli.md`](docs/cli.md).

`AGENTS.md` is create-only: `ahm init` can add the starter entrypoint when it
is missing, but `ahm` never overwrites an existing project `AGENTS.md`.

## Safety

`ahm` does not run git commits, pushes, PRs, or source-code patches itself.
Write commands are explicit and operate on the `.agents` workflow files unless a
future command states otherwise. `ahm task work <id>` is an explicit delegation
command: it validates the task workflow state, then invokes the selected
external coding-agent CLI from the repository root. With `--commit`, `ahm`
resumes the delegated agent session and asks that agent to commit the completed
work; the delegated agent and project hooks own the actual git operation.

## Development

For the full local setup, command catalog, verification expectations, and
commit conventions, see [`CONTRIBUTING.md`](CONTRIBUTING.md). For the module
map and architectural invariants, see [`ARCHITECTURE.md`](ARCHITECTURE.md).
For the documentation map, see [`docs/README.md`](docs/README.md).

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
