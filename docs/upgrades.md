# Workflow Upgrades

`ahm` owns the workflow templates. To update the workflow, edit files under
`internal/templates/workflow/`, rebuild `ahm`, and run:

```bash
ahm upgrade
```

The upgrade process compares the installed metadata in `.agents/ahm.json` with
the target repository files.

- Missing managed files are created.
- Files that still match the previous managed hash are updated.
- Files with local modifications are preserved and reported as conflicts.
- `AGENTS.md` is create-only. `ahm` may add the starter entrypoint when it is
  missing, but it never overwrites an existing `AGENTS.md`, even with
  `--force`.
- Generated indexes are regenerated.
- User-owned task files, research notes, and ExecPlans are not overwritten.

Use `--dry-run` to preview changes. Use `--force` only when the embedded
template should replace local edits to managed workflow files.
