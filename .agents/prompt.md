You are implementing one task in a coordinated architectural shift:
ahm is becoming the runtime for agent work in a repository — state,
procedure, and enforcement delivered as commands instead of installed
files. The whole is described in docs/VISION.md. Your task is one piece
of it; when your task and the vision seem to disagree, the vision's
design tests are the tiebreaker, and disagreement is a blocker to
raise, not a call to improvise.

## Orient (before writing code)

1. Run `ahm context task`, then `ahm task show <id>` and read the full
   task file INCLUDING comments — comments carry cross-tree constraints
   added after the task was written.
2. Read docs/VISION.md (short). Then read the ADR your tree hangs off:
   ADR 013 for the records tree (138–145), ADR 014 for the
   prime/procedures tree (task 156a), the task-160a ADR for the
   docs-check tree. If that ADR is not yet accepted, stop — your task
   should be Blocked.
3. Classify the implementation under AGENTS.md Workflow Routing and
   load only the routed docs.

## Invariants (vision design tests — hold every change against them)

- Git-safety boundary: read git freely; write workflow records under
  tool-owned `.ahm/` and refs under `refs/ahm/*` (and move files out of
  `.agents/` only during explicit opt-in migration); never commit,
  stage, touch the index or HEAD, mutate branches, open PRs, or patch
  project source. VISION.md is the canonical statement — if your change
  needs more than it allows, stop and flag; never renegotiate the
  boundary locally.
- Hook-grade commands (`ahm prime`, `ahm docs check`) must be fast,
  idempotent, and offline-tolerant: a failed sync or check degrades to
  a warning, never a blocked session.
- One structure, three renders: text, --plain, and --json come from
  the same struct (textRenderer pattern, internal/output).
- Advisory stays advisory: findings that mean "may need review" never
  affect exit codes, including under --strict.
- Prefer a command over an installed file. Keep workflow ceremony out
  of branch history and durable knowledge in it.
- Anything requiring judgment stays out of the binary — checks must
  run from hooks without judgment calls; judgment stays in agent skills
  or project docs.

## Coordinate (this is where parallel trees collide)

- The deprecations (managed skills, unscoped `ahm context`,
  `ahm agents suggestions`, `--check project-docs`, committed records)
  land as ONE migration with one upgrade guide and onward
  pointers; do not remove a surface ahead of the coordinated plan.
- Shared guidance docs (AGENTS.md, the workflow spec) are rewritten by
  tasks 145, 156g, and 160d. Before editing any of them, check the
  other trees' status, and note in your handoff what you touched so the
  others can rebase their plans.
- Never hand-edit generated indexes; update the source records and run
  the appropriate `ahm` command.
- Record decisions, surprises, and discovered constraints as task
  comments (`ahm task comment <id>`) so the cross-tree records
  survive.

## Finish

- Update the docs your change makes stale in the same change (your
  task lists them); run the verifications CONTRIBUTING.md prescribes.
- If you hit a design hole or an inter-tree conflict: comment on the
  task, set it Blocked with the reason and hand off — an
  honest blocker beats invented architecture.
- Hand off per AGENTS.md: route and docs loaded, what changed,
  exact checks run, remaining risk.
