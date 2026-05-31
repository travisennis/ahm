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
`ahm task start`, `ahm task complete`, or `ahm task cancel` unless you edit
metadata by hand afterward; those commands already regenerate indexes.

Use `ahm task complete <id>` and `ahm task cancel <id>` for task state
transitions that move files between task buckets.

Do not commit or push unless explicitly asked.

When moving implementation between files or packages, update repository code
maps and implementation-location references even if user-facing behavior is
unchanged.
