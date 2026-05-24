# ahm Initial Project Plan

## Summary

Build `ahm` as a self-contained Go CLI that fully replaces
`agent-workflow-scaffold` as the owner of the agent workflow system. The
workflow contract starts from the scaffold model, but future workflow files,
templates, migrations, and updates live in `ahm`.

`ahm` installs, manages, validates, and upgrades `.agents` workflows in local
repositories. It combines the `.agents` workflow model, task-management
ergonomics from `managing-tickets`, and a polished CLI shape inspired by
`clawpatch`.

## Key Changes

- Create a Go CLI named `ahm`.
- Embed canonical workflow templates in the binary.
- Add install, upgrade, status, doctor, index, and task commands.
- Preserve repo-local workflow state under `.agents/`.
- Keep tasks as Markdown files with YAML-style front matter.
- Use explicit write commands only; no commits, pushes, PRs, or source edits.

## Workflow Template And Upgrade Model

- Store canonical workflow files under `internal/templates/workflow/`.
- Embed templates with Go `embed`.
- Record installed metadata in `.agents/ahm.json`.
- `ahm init` installs missing files and records the template version.
- `ahm upgrade` updates files previously written by `ahm` when they have not
  been locally modified.
- User-owned task files, research notes, and ExecPlans are preserved.
- Generated indexes are regenerated instead of merged by hand.
- `--dry-run` previews install and upgrade writes.

## Interfaces And Data Model

Task files remain Markdown with front matter:

- `id`
- `title`
- `status`
- `priority`
- `effort`
- `labels`
- `exec_plan`
- `depends_on`
- optional `created`, `updated`, `parent`, `external_ref`

Supported statuses are `Open`, `Pending`, `In Progress`, `Blocked`, `Tracking`,
`Completed`, and `Cancelled`. Supported priorities are `P0` through `P4`.
Supported effort values are `XS`, `S`, `M`, `L`, and `XL`.

A task is ready when status is `Pending` and all dependencies are completed.

## Test Plan

- Unit tests for front matter parsing, task sorting, next-ID allocation,
  ready/blocked detection, dependency tree traversal, cycle detection, template
  manifest loading, and upgrade decisions.
- Golden tests for generated Markdown indexes and scaffold install output.
- Integration tests for `ahm init`, `ahm upgrade`, task lifecycle commands, and
  dependency commands.
- Validation commands: `go test ./...`, `go vet ./...`, and `gofmt`.

## Assumptions

- `ahm` is implemented as a Go CLI.
- Canonical workflow files live in this repository and are embedded into
  released binaries.
- Updating workflow files means editing templates in this repository and
  rebuilding or releasing `ahm`.
- v1 does not call coding agents or model providers.
