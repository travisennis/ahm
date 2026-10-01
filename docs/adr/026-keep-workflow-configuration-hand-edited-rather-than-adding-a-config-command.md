---
status: accepted
date: 2026-10-01
decision-makers: Travis Ennis
---
# Keep workflow configuration hand-edited rather than adding a config command

## Context and Problem Statement

`ahm` stores repository-scoped workflow settings in the committed
`.ahm/config.json` file. There is no `ahm config` command: the file is read by
every command and edited by hand. Task 155 asks whether this reduced surface
justifies one, and what its grammar and ownership boundary should be if so.

The surface is small. `internal/ahm/install.go` defines the `metadata` type
with four ahm-recognized keys and an unknown-field passthrough:

- `version` — the obsolete template-version field. Retained only so existing
  files round-trip; fresh metadata omits it, and `ahm` never reads or writes a
  value.
- `strict_acceptance` — a boolean that defaults to `false`. When it is `true`,
  `ahm task complete <id>` fails if the acceptance section is missing, still
  contains the seeded placeholder, or contains unchecked items
  (`internal/ahm/task_status.go`). This is the only key a consumer changes for
  behavior.
- `tasks_location` — the records layout (`home` or project). It is written by
  `ahm init` and `ahm store migrate --to home|project`, not by hand.
- `files` — the managed-file ownership hash map inherited from older releases.
  This version records and validates no new hashes; `ahm init` only
  relinquishes entries it knows.

Every other top-level key is an unknown field. `metadata.UnmarshalJSON`
preserves unknown keys in `Extra`, and `MarshalJSON` re-emits them in sorted
order, so ahm-owned writes never drop project-owned extension fields.

Two facts shape the decision. First, after ADR 022 the surface that remains is
a single behavior-bearing setting. Second, the file is already validated:
`ahm status`, `doctor`, and `prime` report `metadata_missing` and
`metadata_corrupt`, and every command fails loudly when the file is unreadable
rather than silently defaulting. ADR 022 explicitly narrowed `ahm` to
mechanical record operations and removed the surfaces that carried no
mechanical responsibility, so any new command has to clear that bar. The
historical two-key proposal in task 155 (`taskWork`, `research`) was written
against commit `ff15056` and names keys ADR 022 removed; it is history, not a
scope to revive.

## Decision Drivers

- Keep the CLI boundary ADR 022 drew: `ahm` owns parsing, storing, indexing,
  and validating structured records, not convenience wrappers around settings
  it already documents.
- Do not add command, flag, parity, documentation, and test surface whose only
  job is to replace a one-line hand edit of a single boolean.
- Recognize that metadata validation already exists in `status`, `doctor`, and
  `prime`; a validating config surface would duplicate it.
- Preserve write integrity: any writer must round-trip unknown fields and must
  never rewrite the ahm-owned `version`, `tasks_location`, or `files` keys.
- Keep the failure mode loud and the fix obvious: corrupt metadata already
  blocks every command and is repaired by `ahm init`.
- Prefer a decision the maintainer can reverse cheaply if discovery, not
  writing, turns out to be the real gap.

## Considered Options

- **Typed fixed-key surface.** `ahm config get/set/unset/list` over a closed
  mutable vocabulary. Only `strict_acceptance` is writable; `get` and `list`
  read effective values; `unset` has no meaning because the key is a boolean
  with a default.
- **Whole-document edit/validate workflow.** `ahm config` opens or validates
  the document as a whole: an editor spawn, a schema or format check, or both.
- **No command.** Keep hand-editing `.ahm/config.json` as the supported write
  path, with validation remaining in `status`, `doctor`, and `prime`, and
  narrow the discovery gap inside those existing surfaces.

## Decision Outcome

Chosen option: **no dedicated configuration command**, because the surviving
mutable surface is a single boolean, the file is already validated by existing
diagnostics, and a new command would add permanent CLI, documentation, parity,
and test surface for less value than the one-line edit it replaces.

The typed surface is rejected as over-built: a `get`/`set`/`unset`/`list` verb
set over one settable key is more interface than payload, and `unset` is
meaningless for a key whose absence already means `false`. The whole-document
workflow is rejected because its validating half duplicates `status` and
`doctor`, and its editing half would have to either spawn a user editor — an
arbitrary command execution this tool deliberately avoids — or accept
free-form JSON, which is the hand edit it claims to improve.

### What stays true

- **Mutable vocabulary.** `strict_acceptance` (boolean, default `false`) is the
  only key a consumer changes for behavior. It is documented in
  `docs/references/workflow-spec.md`.
- **Ahm-owned keys.** `version`, `tasks_location`, and `files` are owned by
  `ahm` and are not hand-edited settings. `version` is preserved for round-trip
  only; `tasks_location` is changed through `ahm store migrate --to
  home|project`, never by editing the file; `files` is reconciled by
  `ahm init`.
- **Unknown-field preservation.** Unknown top-level keys are project-owned
  extension data. Any write path, including a hand edit, must leave them
  intact; `ahm` already guarantees this for its own writes by round-tripping
  `Extra` in sorted order.
- **Missing or corrupt metadata.** A missing `.ahm/config.json` means
  uninstalled and is reported as `metadata_missing` (exit 1) with `ahm init` as
  the fix. A corrupt file is reported as `metadata_corrupt` (exit 1) and fails
  every command loudly. `ahm init` reconciles present metadata but does not
  silently create, reset, or repair a corrupt document into defaults.
- **Safety.** No command executes an external editor or shell, and no path
  creates or resets metadata implicitly.

### Consequences

- Good, because the CLI boundary stays where ADR 022 put it: no new command,
  alias, flag, inventory entry, or output and exit-code contract to maintain.
- Good, because validation is not duplicated; `status`, `doctor`, and `prime`
  remain the single surface that reports metadata problems, and the failure
  stays loud with an obvious `ahm init` remediation.
- Bad, because discovery is weak: nothing in `status` currently prints the
  effective `strict_acceptance` value, so a consumer must read
  `docs/references/workflow-spec.md` or the file to learn it exists.
- Bad, because a hand edit can still introduce invalid JSON or drop an unknown
  field, and no ahm command guards against it; the failure surfaces only when
  the next command reads the file.
- Neutral, because the mutable vocabulary and prohibited keys are unchanged;
  this decision documents them rather than adding behavior.

## More Information

- `docs/references/workflow-spec.md` remains the authority for
  `.ahm/config.json` ownership and supported settings; it already states that
  `strict_acceptance` defaults to `false` and describes the `files` map's
  retirement.
- ADR 022 defines the boundary this decision preserves and removed the keys the
  historical task-155 proposal named.
- ADR 001 governs the atomic write any future configuration writer would have
  to use.

### Follow-up work if this decision is accepted

These are described, not created here:

1. **Documentation (workflow spec).** Tighten the `.ahm/config.json` section of
   `docs/references/workflow-spec.md` so it states the exact mutable and
   prohibited key split: `strict_acceptance` mutable; `version`,
   `tasks_location`, and `files` ahm-owned; unknown keys project-owned and
   preserved.
2. **Implementation (discovery in `status` and `doctor`).** Report the effective
   `strict_acceptance` value and the resolved records mode in `status` and
   `doctor`, so the setting is discoverable from the tool instead of only from
   documentation. This is the smallest change that closes the discovery gap
   without adding a command.
3. **Documentation (CLI reference note).** Add a sentence to
   `docs/references/cli/commands.md` that `.ahm/config.json` is hand-edited and
   validated by `status` and `doctor`, so the absence of a `config` command is
   intentional and discoverable.

### If the maintainer reverses this decision

The smallest command model that would justify a command, and the contract it
must meet, is:

- **Surface.** `ahm config` (read: print effective settings and validation) and
  `ahm config set <key> <value>`. Add `get` or `list` only if asked for; do not
  add `unset`, because no mutable key is nullable.
- **Addressing.** Dot-free, top-level, lowercase keys matching the JSON field
  names (`strict_acceptance`). No nested or dotted paths.
- **Mutable vocabulary.** `strict_acceptance` only. `version`,
  `tasks_location`, and `files` are read-only through the command and rejected
  as write targets with a usage error (exit 2). Unknown keys are neither
  readable nor writable and are preserved on write.
- **Value validation.** `strict_acceptance` accepts `true` or `false` only; any
  other value is a usage error (exit 2) naming the accepted values.
- **Output.** Text by default (`strict_acceptance = true`); `--json` prints the
  key and value plus the resolved records mode; `--plain` prints compact
  single-line JSON. `set` prints `<key>: <from> -> <to>`.
- **Dry-run.** `config set --dry-run` computes and prints the change without
  writing; read paths ignore it.
- **Exit codes.** `0` success; `2` unknown or immutable key, or invalid value;
  `1` missing or corrupt metadata, or a write failure.
- **Atomic write.** Write through the existing atomic-write helper under the
  workflow record lock, re-emitting `version`, `strict_acceptance`,
  `tasks_location`, and `files` in canonical order followed by sorted unknown
  fields.
- **Missing or corrupt metadata.** `config` reads report `metadata_missing` or
  `metadata_corrupt` (exit 1); `config set` refuses both rather than creating
  or resetting the file, and names `ahm init` as remediation.
- **Safety.** Never spawn a shell or editor; never create or reset metadata
  implicitly; never write an ahm-owned key.
