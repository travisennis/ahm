# Agent Instructions

Before creating, choosing, updating, or working on tasks, read
`.agents/TASKS.md`. Use `.agents/.tasks/index.md` as the generated task queue,
then open the specific task file before acting.

Before creating, updating, organizing, or using research, read
`.agents/RESEARCH.md`. Use `.agents/.research/index.md` as the research map.

For large work, read `.agents/PLANS.md`. Tasks marked `L` or `XL` require an
ExecPlan before implementation.

Do not edit generated indexes by hand. Update source task files and run
`ahm index`.

Use `ahm task complete <id>` and `ahm task cancel <id>` for task state
transitions that move files between task buckets.

Do not commit or push unless explicitly asked.
