# ahm CLI Reference

This document describes the supported `ahm` commands, flags, outputs, and
write behavior. The executable entrypoint is `cmd/ahm/main.go`; command wiring
lives in `internal/ahm/cli.go`, with focused implementation files under
`internal/ahm/`.

## Usage

```bash
ahm [global flags] <command> [command flags]
```

When no command is provided, `ahm` runs `status`.

Exit codes:

- `0`: success.
- `1`: runtime failure. `status` and `doctor` use exit code 1
  when the workflow validation report contains errors, without printing
  `error:` to stderr.
- `2`: invalid usage, such as an unknown flag or missing required argument.

## Root Selection

Most commands operate on a target repository root.

By default, `ahm` walks upward from the current working directory until it finds
a `.git` directory, `.ahm/config.json`, or `.agents/ahm.json`. If none are
found, the command fails with an error message that explains how to use
`--root` or `ahm init`.

Use `--root <path>` to bypass auto-detection and operate on a specific
directory.

`init`, `upgrade`, and `agents suggestions` are lenient: they can run in any
directory. `init` creates the `.agents` workflow scaffolding, `upgrade`
refreshes it, and `agents suggestions` only prints advisory text. `prime`,
`context`, and all other state-aware commands require a managed repository
(`.git`, `.ahm/config.json`, or `.agents/ahm.json`).

## Global Flags

Global flags must appear before the command.

| Flag | Description |
| ---- | ----------- |
| `--root <path>` | Sets the target repository root. Defaults to the nearest git root, `.ahm/config.json` parent, or `.agents/ahm.json` parent. Outside a managed repository, strict commands fail with remediation instructions; use `--root` to bypass auto-detection. |
| `--json` | Emits structured JSON for commands that use the shared emitter. For task list/show commands, this returns parsed task structs with lowercase snake_case keys (`id`, `title`, `status`, `priority`, etc.). Takes precedence over `--plain` and `--text`. |
| `--plain` | Emits stable line-oriented output for shared-emitter responses by printing compact JSON on one line. Ignored by commands with custom text output. Takes precedence over `--text`. |
| `--text` | Emits human-friendly text output. This is the default mode. The flag exists for explicit clarity in scripts but does not override `--json` or `--plain`. |
| `--dry-run` | Previews supported write operations without writing files. Supported by `init`, `upgrade`, `index`, `adr create`, ADR lifecycle commands, `records migrate`, `records pull`, `records push`, `records sync`, `task create`, `task work`, `task migrate`, task status transitions, and task dependency add/remove. |
| `--force` | Forces supported removals during `upgrade`, and overrides strict acceptance checks during `task complete`. It never creates, overwrites, or removes `AGENTS.md`. |
| `--help`, `-h` | Prints command help. |
| `--version` | Prints the ahm binary version. |

Examples:

```bash
ahm --root /path/to/repo status
ahm --json doctor
ahm --dry-run upgrade
```

## Output Modes

ahm supports three output modes: text (default), JSON (`--json`), and compact
JSON (`--plain`). Precedence: `--json` takes priority over `--plain`, and
`--plain` takes priority over the default text mode. The `--text` flag selects
the default explicitly and does not override `--json` or `--plain`.

In the default text mode, structured commands such as `status` and `doctor`
print human-friendly key-value output:

```text
root: /path/to/repo
template_version: 1.0.0
installed: true
installed_version: 1.0.0
tasks:
  total: 5
  pending: 2
  in_progress: 1
  completed: 2
validation:
  ok: true
  errors: 0
  warnings: 0
```

When the workflow metadata is missing (not yet installed), `installed_version`
shows as `none` in text mode and `null` in JSON/plain mode, and the
validation report includes the metadata error:

```text
root: /path/to/repo
template_version: 1.0.0
installed: false
installed_version: none
tasks:
  total: 0
  pending: 0
  in_progress: 0
  completed: 0
validation:
{
    "ok": false,
    "errors": [
      {
        "code": "metadata_missing",
        "path": ".agents/ahm.json",
        "message": "workflow metadata is missing"
      }
    ],
    "warnings": [],
    "info": []
  }
```

Install and upgrade operations always print grouped text sections such as
`adopted:`, `created:`, `updated:`, `skipped:`, and `conflicts:`.

Some task commands use command-specific text output regardless of the output
mode:

- `agents suggestions` prints advisory agent instructions as Markdown unless
  `--json` is used.
- `adr create` prints the created ADR ID.
- `task create` prints the created task ID.
- `task list`, `task ready`, `task blocked`, and `task next` print task lines.
- `task labels` prints label summary lines.
- `task show` prints the task Markdown file unless `--json` is used.
- `task migrate --dry-run` prints grouped task migration changes.
- Task status transitions print `<id> -> <status>`; if the task already has the target status, prints `<id> already <status>` instead and skips writing.
- Dependency updates print `<id> depends_on: <dependencies>`; if the dependency is already present (add) or absent (remove), prints `<id> already depends on <dep>` or `<id> does not depend on <dep>` instead and skips writing.
- Dependency tree and cycle commands print tree/path text.
