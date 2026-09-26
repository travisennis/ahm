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
a `.git` directory or `.ahm/config.json`. If neither is found, the command
fails with an error message that explains how to use `--root` or `ahm init`.
A repository whose metadata is still the retired `.agents/ahm.json` is refused
with an error that names the final v1 release (`v1.1.0`) to upgrade with first;
`ahm` never treats such a repository as unmanaged.

Use `--root <path>` to bypass auto-detection and operate on a specific
directory.

## Home Store Environment

Task records in home mode are stored outside the repository under `~/.ahm`.
Set `AHM_HOME` to override the store root with an absolute path; a relative
value is a usage error. The store root contains `registry.json` and
`projects/<slug>-<hash8>/` directories. `ahm store path` reports the resolved
root, project key, and records directory. Home-mode identity uses the remote
Git selects: `origin` when present, otherwise the only remote when the
repository has exactly one. A project whose root contains `.git` requires
readable Git state; a project root without `.git` uses the path rule and reads
no Git. See
[the workflow specification](../workflow-spec.md) for the store format and
ownership boundary.

`init` is lenient: it can run in any directory and creates the `.ahm` workflow
scaffolding there. `status` reports a git repository that has no
`.ahm/config.json` as `installed: false`, provided Git can read the repository:
a repository with no configuration is a new project, so it resolves the store,
and a `.git` directory Git cannot read (or a `git` missing from `PATH`) fails
the command with the resolver's error instead of reporting. Outside a managed
repository, with neither `.git` nor `.ahm/config.json` above the working
directory, `status`, `prime`, `doctor`, and the `task`, `adr`, and `store`
commands all fail with remediation instructions; `--root` bypasses
auto-detection for any of them.

## Global Flags

Global flags must appear before the command.

| Flag | Description |
| ---- | ----------- |
| `--root <path>` | Sets the target repository root. Defaults to the nearest git root or `.ahm/config.json` parent. Outside a managed repository, strict commands fail with remediation instructions; use `--root` to bypass auto-detection. |
| `--json` | Emits structured JSON for commands that use the shared emitter. For task list/show commands, this returns parsed task structs with lowercase snake_case keys (`id`, `title`, `status`, `priority`, etc.). Takes precedence over `--plain` and `--text`. |
| `--plain` | Emits stable line-oriented output for shared-emitter responses by printing compact JSON on one line. Ignored by commands with custom text output. Takes precedence over `--text`. |
| `--text` | Emits human-friendly text output. This is the default mode. The flag exists for explicit clarity in scripts but does not override `--json` or `--plain`. |
| `--dry-run` | Previews supported write operations without writing files. Supported by `init`, `index`, `adr create`, ADR lifecycle commands, `task create`, task status transitions, task dependency add/remove, `store path`, and `store migrate`. |
| `--force` | Overrides strict acceptance checks during `task complete`, the refusal of `store migrate --to home` when the project's records have uncommitted changes, and the refusal of a `store migrate` whose destination record differs from the record arriving. It never creates, overwrites, or removes `AGENTS.md`. |
| `--help`, `-h` | Prints command help. |
| `--version` | Prints the ahm binary version. |

Examples:

```bash
ahm --root /path/to/repo status
ahm --json doctor
ahm --dry-run init
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
installed: true
installed_version: 1.0.0
store:
  root: /Users/you/.ahm
  key: github.com/owner/repo
  kind: remote
  location: home
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

The `store` block is present in `status` and `prime` only for an installed
project whose records live in the home store. It is absent in project mode and
for an uninstalled repository; JSON and plain output carry the same
`root`, `key`, `kind`, and `location` fields. `ahm store path` is the
deliberate exception: it reports absolute store paths, and text output
abbreviates the user's home directory to `~`.

When the workflow metadata is missing (not yet installed), `installed_version`
shows as `none` in text mode and `null` in JSON/plain mode, and the
validation report includes the metadata error:

```text
root: /path/to/repo
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
        "path": ".ahm/config.json",
        "message": "workflow metadata is missing"
      }
    ],
    "warnings": [],
    "info": []
  }
```

`init` prints grouped text sections such as `created:`, `updated:`, `directories:`,
and `indexes:`; an up-to-date repository reports none of them.

Some task commands use command-specific text output regardless of the output
mode:

- `adr create` prints the created ADR ID.
- `task create` prints the created task ID.
- `task list`, `task ready`, `task blocked`, and `task next` print task lines.
- `task labels` prints label summary lines.
- `task show` prints the task Markdown file unless `--json` is used.
- Task status transitions print `<id> -> <status>`; if the task already has the target status, prints `<id> already <status>` instead and skips writing.
- Dependency updates print `<id> depends_on: <dependencies>`; if the dependency is already present (add) or absent (remove), prints `<id> already depends on <dep>` or `<id> does not depend on <dep>` instead and skips writing.
- Dependency tree and cycle commands print tree/path text.
