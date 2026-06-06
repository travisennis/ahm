# Agent Instructions

For the first task in a session, read `.agents/TASKS.md`, then use
`.agents/.tasks/index.md` as the generated task queue and open the specific
task file. For later tasks in the same session, reread only the task index
and specific task file unless `.agents/TASKS.md` changed or the task changes
task workflow semantics.

Before creating, updating, organizing, or using research, read
`.agents/RESEARCH.md`. Use `.agents/.research/index.md` as the research map.

For large work, read `.agents/PLANS.md`. Tasks marked `L` or `XL` require an
ExecPlan before implementation.

Before auditing or updating documentation, read `.agents/DOCS.md`. Prefer the
target repository's existing documentation conventions over adding new
structures.

Do not edit generated indexes by hand. Update source task, research, or
ExecPlan files and run `ahm index`. Do not run `ahm index` after
`ahm task create`, `ahm task start <id>`, `ahm task complete <id>`,
`ahm task cancel <id>`, or `ahm task reopen <id>` unless you edit metadata
by hand afterward; those commands already regenerate indexes.

`ahm` owns the workflow files it installs, maintains, generates, and upgrades.
Do not hand-edit ahm-owned generated indexes or managed templates in a consumer
project to change ahm behavior or guidance. Update the source task, research
note, ExecPlan, or other project-owned record, then run the appropriate `ahm`
command (`ahm index`, `ahm task complete <id>`, `ahm upgrade`, etc.). If the
installed guidance itself needs to change, update the canonical templates in
the `ahm` repository and let projects receive the change through `ahm upgrade`.

Exceptions: `AGENTS.md` is project-owned after creation; task files, research
notes, ExecPlans, and ADRs are workflow source records that may be updated
through their documented workflows; generated indexes must be regenerated, not
hand-edited; managed template files under `.agents/` and `docs/adr/README.md`
should not be customized locally as a way to change ahm-provided process
guidance.

Use `ahm task complete <id>` and `ahm task cancel <id>` for task state
transitions that move files between task buckets.

Do not commit or push unless explicitly asked.

When moving implementation between files or packages, update repository code
maps and implementation-location references even if user-facing behavior is
unchanged.
