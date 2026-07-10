# ahm Task Commands

This reference covers task lifecycle, dependency, delegation, completion,
cancellation, and reopening commands. For task file and validation formats, see
[task file and validation formats](task-file-format.md).

## Task Commands

Task commands operate on Markdown task files in the current storage mode:

- `.agents/.tasks/active|completed|cancelled/` in legacy committed-record repositories.
- `.ahm/tasks/active|completed|cancelled/` after ref-backed migration.

Task IDs are resolved by exact string match first. If no exact match is found, an exact numeric match is attempted: the pattern and task ID are parsed by numeric value and optional letter suffix, so `1` matches `001` and `1a` matches `001a`. If no exact numeric match exists, numeric prefix matching is used, which can match multiple tasks (e.g., `1` matches both `001` and `001a`). If a prefix matches more than one task, the command lists the matching IDs and fails as ambiguous.

All task front matter fields are preserved during status transitions,
dependency edits, and other task mutations. Known fields (`id`, `title`,
`status`, `priority`, `effort`, `labels`, `exec_plan`, `depends_on`,
`created`, `updated`, `parent`, `external_ref`) are written in a fixed
order. Unknown fields such as `assignee`, `due`, `tags`, or `ticket` are
preserved and written in sorted key order after the known fields.

Task statuses must be one of:

- `Open`
- `Pending`
- `In Progress`
- `Blocked`
- `Tracking`
- `Completed`
- `Cancelled`

Task priorities must be one of:

- `P0`
- `P1`
- `P2`
- `P3`
- `P4`

Task efforts must be one of:

- `XS`
- `S`
- `M`
- `L`
- `XL`

### Malformed Task Resilience

List-like commands (`task list`, `task ls`, `task ready`, `task blocked`,
`task search`, `task labels`, `task next`, `task dep cycles`, `task dep tree`)
and `ahm index`
tolerate malformed task files. When one or more task files cannot be parsed,
these commands skip the malformed files, produce output from the remaining
valid tasks, and print a warning to stderr.

`task create` also tolerates malformed task files: it warns on stderr and
still assigns the next available ID, scanning both parsed tasks and task
files on disk to avoid ID collisions.

Task resolution commands (`task show`, `task work`, `task start`,
`task complete`, `task cancel`, `task accept`, `task reopen`, `task comment`,
`task dep add`, `task dep remove`) also
skip malformed files during ID resolution. A malformed task cannot be
resolved by ID and produces a `task not found` error.

Validation commands (`ahm status`, `ahm doctor`) are strict: they report
malformed task files as `task_malformed` validation errors and exit with
code 1.

To recover from a malformed task file, inspect the file, fix the front
matter (missing required fields, invalid enum values, or parse errors
such as unsupported block scalars), and run `ahm status` or `ahm doctor`
to confirm the repair.

### `task create <title> [flags]`

Creates a new active task and regenerates indexes.

**Top-level ID allocation:** The next ID is the next zero-padded numeric ID
after the highest existing numeric task ID, such as `001`, `002`, and `003`.
Non-numeric suffix IDs are ignored for this calculation.

**Subtask (child) ID allocation:** When the `--parent` flag is provided, the
next available lettered child ID is allocated under that parent. For a parent
with ID `137`, children are named `137a`, `137b`, ..., `137z`. At most 26
children are allowed per parent. The allocation scans parsed tasks and
filesystem entries across `active/`, `completed/`, and `cancelled/` buckets to
avoid collisions with existing children.

Concurrent `task create` commands in the same repository are serialized with a
repo-local workflow lock while the ID is allocated, the task file is written,
and indexes are regenerated. This prevents parallel creates from receiving the
same numeric ID or leaving task indexes stale.

The title is built from all non-flag arguments, so both of these are valid:

```bash
ahm task create "Add release workflow" --priority P1
ahm task create Add release workflow --priority P1
```

Command flags:

| Flag | Description |
| ---- | ----------- |
| `--priority <value>`, `-p <value>` | Sets task priority. Default is `P2`. |
| `--effort <value>` | Sets task effort. Default is `S`. |
| `--labels <value>` | Sets the raw labels front matter value. Default is `type:task, area:unknown`. |
| `--status <value>` | Sets initial task status. Default is `Open`. |
| `--description <text>`, `-d <text>` | Sets the initial summary body. Default is `TODO.` |
| `--body-file <path>` | Reads the task body from a file, or from stdin when the path is `-`. |
| `--parent <id>` | Parent task ID for subtask creation. Allocates a suffixed child ID (e.g., `137a`) and writes `parent: <id>` in front matter. The parent must be a top-level task (no letter suffix); child tasks cannot be parents. |

By default the created task has `exec_plan: -`, no dependencies, a `## Summary`
section, and a `## Acceptance Notes` checklist.

`--body-file` provides the full Markdown body that appears after the generated
H1 title. `ahm` still owns ID allocation, front matter, the `# <title>` heading,
the active task location, and index regeneration; only the body content below
the H1 is taken from the file. The file content is whitespace-trimmed and CRLF
line endings are normalized to LF.

If the body file starts with an `# <title>` line that matches the task title,
it is automatically stripped to avoid a duplicate top-level heading. A
different H1 is preserved as intentional body content.

```bash
ahm task create "Add JSON Output To task list" \
  --priority P2 \
  --effort M \
  --labels "type:feature, area:cli" \
  --body-file -
```

`--body-file` and `--description` are mutually exclusive. The command reports an
explicit error when the body file cannot be read, when stdin is requested but
unavailable, or when the resolved body is empty.

Useful global flags:

- `--dry-run`: prints the target path and ID without creating the task.
- `--json` or `--plain`: affects only dry-run output. Successful non-dry-run
  creation prints the task ID.

Examples:

```bash
ahm task create "Add release workflow" --priority P2 --effort M --labels type:task,area:ci

ahm task create "Implement auth" --parent 047 --status Pending \
  --labels "type:feature, area:config" --priority P1
```

### `task list`

Lists parsed tasks.

Alias:

- `task ls`

Text output is sorted by priority rank and then task ID:

```text
001 [Pending] P2 S Add release workflow
```

Useful flags:

- `--status <status>`: filters tasks by one or more statuses. Accepts a
  comma-separated list (`--status pending,completed`) or repeated flags
  (`--status pending --status completed`). Status matching is per-entry
  case-insensitive and accepts `in-progress` for `In Progress`.
  Duplicate statuses are ignored.
- `--label <label>`: filters tasks by one or more labels. Accepts a
  comma-separated list (`--label type:feature,area:cli`) or repeated flags
  (`--label type:feature --label area:cli`). Matching uses AND semantics:
  every supplied label must be present on the task.
- `--priority <priority>`: filters tasks by one or more priorities. Accepts a
  comma-separated list (`--priority P0,P1`) or repeated flags
  (`--priority P0 --priority P1`). Matching is case-insensitive.
  Duplicate priorities are ignored.
- `--effort <effort>`: filters tasks by one or more efforts. Accepts a
  comma-separated list (`--effort S,M`) or repeated flags
  (`--effort S --effort M`). Matching is case-insensitive. Duplicate efforts
  are ignored.
- `--json`: emits parsed task structs with lowercase snake_case keys (`id`, `title`, `status`, `priority`, `effort`, `labels`, `exec_plan`, `depends_on`, `created`, `updated`, `parent`, `external_ref`, `extra`, `path`, `bucket`, `body`).

When multiple filters are supplied, `task list` applies AND semantics across
filter types.

Example:

```bash
ahm task list
ahm task list --status pending
ahm task list --status pending,completed
ahm task list --status open --status "in progress"
ahm task list --label type:feature --label area:cli
ahm task list --priority P0,P1 --effort S,M
```

### `task ready`

Lists pending tasks whose dependencies are all completed.

Completed dependencies are determined by dependency task status, not by file
bucket alone.

Useful flags:

- `--label <label>`: filters ready tasks by one or more labels. Matching uses
  the same AND semantics as `task list --label`.
- `--json`: emits parsed task structs with lowercase snake_case keys.

Example:

```bash
ahm task ready
ahm task ready --label area:cli
```

### `task blocked`

Lists blocked tasks.

A task is considered blocked when either:

- Its status is `Blocked`.
- Its status is `Pending` and at least one dependency is not completed.

Useful flags:

- `--label <label>`: filters blocked tasks by one or more labels. Matching uses
  the same AND semantics as `task list --label`.
- `--json`: emits parsed task structs with lowercase snake_case keys.

Example:

```bash
ahm task blocked
ahm task blocked --label risk:external-service
```

### `task search <query>`

Searches tasks by case-insensitive substring match on the task title. Body
text is not searched.

Text output matches `task list`, sorted by priority rank and then task ID:

```text
001 [Pending] P2 S Add release workflow
```

Useful flags:

- `--status <status>`: filters matches by one or more statuses, using the same
  semantics as `task list --status`.
- `--label <label>`: filters matches by one or more labels, using the same AND
  semantics as `task list --label`.
- `--json`: emits parsed task structs with lowercase snake_case keys.

When filters are supplied, they compose with the title search using AND
semantics. Empty results print `No tasks found.` in text mode and `[]` in JSON
mode. Calling `task search` with no query is a usage error.

Example:

```bash
ahm task search timeout
ahm task search "release workflow"
ahm task search timeout --status Open
ahm task search cli --status Open --label area:cli
ahm --json task search cli --label area:cli
```

### `task labels`

Lists labels currently used by parsed task files. Text output is sorted by
label and includes total task count, active-bucket count, `Open` status count,
and ready task count:

```text
area:cli total=7 active=4 open=0 ready=2
```

Useful flags:

- `--json`: emits label summary objects with `label`, `total`, `active`,
  `open`, and `ready` fields.

Example:

```bash
ahm task labels
ahm --json task labels
```

### `task next`

Shows the first ready task by the same ordering used by `task ready`: priority
rank first, then task ID. A task is ready when its status is `Pending` and all
dependencies are completed.

Useful flags:

- `--json`: emits the parsed task struct with lowercase snake_case keys, or `null` when no task is ready.

Example:

```bash
ahm task next
```

### `task migrate`

Normalizes legacy task front matter for projects that used an ahm-like workflow
before adopting the current ahm schema.

The migration is intentionally mechanical. It can:

- Add missing `labels` as `type:task, area:unknown`.
- Convert placeholder `priority: -` to `priority: P3`.
- Convert placeholder `effort: -` to `effort: M`.
- Trim annotated effort values such as `XL (split into subtasks)` to `XL`.
- Trim annotated dependency entries that start with task IDs, such as
  `050 (Backend abstraction), 051 (Tool abstraction)`, to `050, 051`.
- Convert legacy dependency notes such as `Follows 061` or `Completed by 061`
  to their referenced task IDs.

Source-only dependency notes such as `From code review...`, `Resolved in same
PR...`, `Research: ...`, and `Closed as obsolete...` are cleared to `-`.

Useful global flags:

- `--dry-run`: prints the task files and field changes without writing.
- `--json` or `--plain`: emits the migration report in machine-readable form.

Example:

```bash
ahm --dry-run task migrate
ahm task migrate
```

### `task show <id>`

Shows a task.

By default, this prints the raw task Markdown file. With `--json`, it prints the
parsed task struct with lowercase snake_case keys.

Example:

```bash
ahm task show 001
ahm --json task show 001
```

### `task start <id>`

Sets a task status to `In Progress`, moves it to
the active task bucket, removes the old file when the bucket changed, and
regenerates indexes.

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Example:

```bash
ahm task start 001
```

### `task accept <id>`

Sets a task status to `Pending`, stamps `updated`, and regenerates indexes.
This is the intentional transition from `Open` (newly captured, untriaged)
into the ready backlog. The file stays in the active task bucket because both
`Open` and `Pending` live in the same bucket.

Before accepting a task, verify:

- The problem statement is clear and the scope is well defined.
- The relevant files, commands, or modules are identified.
- Labels, priority, and effort are set to reasonable values.
- Upfront dependencies are resolved or documented.
- An ExecPlan exists for `Effort: L` and `Effort: XL` tasks.
- An ADR exists for `type:feature` tasks that introduce durable
  architectural decisions.
- At least a skeleton Acceptance Notes section is present so completion
  criteria are known.

Reasons not to accept a task (leave it `Open` until resolved):

- The scope or problem is vague and needs more discovery.
- Product or design decisions are still outstanding.
- Required dependencies are underspecified or unsatisfiable.
- A required ExecPlan or ADR has not been created yet.

Tasks that are fully scoped at creation can skip the accept step entirely
by using `--status Pending` with `ahm task create`. This is appropriate
when the creator already knows the problem, affected surface, and completion
criteria.

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Examples:

```bash
# Accept a task after triage confirms it is actionable
ahm task accept 001

# Preview the change without writing
ahm --dry-run task accept 001

# Create a fully scoped task that skips the accept step
ahm task create "Fix index sort order" \
  --priority P2 --effort S \
  --labels "type:bug, area:workflow" \
  --description "Tasks list is unsorted; sort by ID ascending." \
  --status Pending
```

### `task work <id> [flags]`

Resolves a task, validates that it can be worked, and hands it to an external
coding-agent CLI from the repository root.

`task work` refuses completed and cancelled tasks. It also verifies every task
listed in `depends_on` is already `Completed` before invoking an agent. If the
task is `Pending`, the command marks it `In Progress`, writes it to
the active task bucket, removes the old file when the bucket changed, and
regenerates indexes after validation and executable lookup, but before invoking
the external CLI.
Tasks already `In Progress`, `Open`, or `Blocked` are not rewritten.

Supported agents:

| Agent | Executable | Invocation | Sessions | Review | Completion | Commit |
| ----- | ---------- | ---------- | -------- | ------ | ---------- | ------ |
| `cake` | `cake` | `cake [--model <name>] --output-format stream-json <prompt>` | Full orchestration | Full orchestration | Full orchestration | Full orchestration |
| `codex` | `codex` | `codex exec [--model <name>] --dangerously-bypass-approvals-and-sandbox --json <prompt>` | Full orchestration | Full orchestration | Full orchestration | Full orchestration |
| `cursor` | `cursor-agent` | `cursor-agent -p [--model <name>] --output-format stream-json --trust <prompt>` | Full orchestration | Full orchestration | Full orchestration | Full orchestration |
| `claude` | `claude` | `claude -p [--model <name>] --verbose --output-format stream-json <prompt>` | Full orchestration | Full orchestration | Full orchestration | Full orchestration |

Agents marked **Full orchestration** for Sessions support session capture and
resume. When such an agent is used, `ahm` requests structured output, captures
the session identifier from the first session-start event (`task_start.session_id`
for `cake`, `thread.started.thread_id` for `codex`,
`system/init.session_id` for `cursor`, `system/init.session_id` for `claude`),
and holds it in memory for the
current invocation. This enables follow-up review, revision, and commit steps
within the same workflow run.

Agents marked **Full orchestration** for Review support independent review
invocation. Review runs by default after the work session. `ahm` runs the repository-owned preflight
review workflow (`.agents/skills/preflight/SKILL.md`) against the current
uncommitted changes, using each agent's normal execution path:

- `cake`: `cake [--model <name>] --skills preflight --output-format stream-json`
- `codex`: `codex exec [--model <name>] --dangerously-bypass-approvals-and-sandbox --json`
  with the preflight prompt
- `cursor`: `cursor-agent -p --output-format stream-json --mode ask --trust`
  with the preflight prompt
- `claude`: `claude -p --verbose --output-format stream-json`
  with the preflight prompt

This means review has consistent semantics across all agents: it runs the
preflight review workflow. If the review produces actionable feedback, `ahm`
resumes the original work session with the feedback and asks the agent to
address each issue. If the review produces no feedback, the feedback-resume
step is skipped. If the review command itself fails, the failure is surfaced
and the command exits with a non-zero code.

When `--no-review` is passed, no review orchestration runs.

Codex is run with `--dangerously-bypass-approvals-and-sandbox` for
non-interactive task work. This is intentionally broad: it avoids sandbox and
approval deadlocks while allowing Codex to edit files, run verification that
writes outside the repository cache, complete tasks, and perform the optional
commit handoff. Only use Codex task work in repositories and working trees where
that trust tradeoff is acceptable.

Agents marked **Full orchestration** for Commit support session-based commit
handoff. Commit runs by default after the work session (and after review
feedback is addressed when review also runs). The commit prompt
asks the delegated agent to commit the completed task work and make sure the
task is marked completed before committing. In legacy committed-record
repositories it asks for both task files and project source files in a single
commit. In ref-backed record repositories (after `ahm records migrate`) task
records are gitignored under `.ahm/` and snapshotted to the records ref by
`ahm` itself, so the prompt instead scopes the commit to project source
changes and tells the agent not to add or force-add gitignored `.ahm/` files.

`ahm` does not run `git commit`, choose commit messages, push branches, or open
pull requests. Commit-message convention is owned by the target project and its
hooks. Pass `--no-commit` to skip the commit handoff.

Agent and model selection precedence for each phase is:

1. `--agent` / `--model` CLI flags (apply to all phases).
2. Role-specific config under `taskWork.implementation` or `taskWork.review`.
3. `.ahm/config.json` `"default_work_agent": "<agent>"` after migration, or
   legacy `.agents/ahm.json` before migration.
4. Built-in default: `"cake"` for agent, no model override.

When no review-specific config is provided, the review phase uses the same
agent as the implementation phase (after applying the full fallback chain).
Feedback-resume and commit handoff always use the implementation agent
because they resume the implementation session.

The generated prompt includes the resolved task ID and instructs
the delegated agent to run `ahm context task`, then run `ahm task show <id>`
to inspect the task before making changes. `ahm` does
not pass provider credentials, choose models, complete tasks, run git commands,
push branches, or open pull requests. With review and commit enabled by default,
`ahm` orchestrates follow-up prompts unless opted out with `--no-review` / `--no-commit`,
but the review, completion, and commit actions
are performed by the delegated agent.

When the file `.agents/prompt.md` exists in the repository root, its content
is appended to the built work prompt under a `## Project Instructions` heading.
This lets the project maintainer carry standing orientation and
company/project-specific instructions that apply to every delegated work session.
The path is configurable via `taskWork.promptFile` in `.ahm/config.json` after
migration, or legacy `.agents/ahm.json` before migration;
a missing or unreadable file is silently ignored — the feature is opt-in
by file presence.

Each phase (work session, review, feedback resume, and commit handoff) has an
independent timeout. If the delegated agent exceeds the timeout for the current
phase, the subprocess is killed and `ahm` exits with a non-zero code.

The default timeout is 30 minutes per phase. Use `--timeout` to override for
long-running tasks — for example, L/XL tasks that need more time for the
initial work session or a lengthy review.

Useful flags:

- `--agent <cake|claude|codex|cursor>`: selects the external coding-agent CLI.
- `--model <name>`: model override for the selected agent. Passes the model to
  the agent via each provider's `--model <name>` CLI flag. Supported by all
  agents (cake, claude, codex, cursor). Affects the initial work session and
  review session; resume and commit handoff reuse the existing session which
  already has the model set.
- `--timeout <duration>`: maximum time for each phase before timeout, in Go
  duration syntax (e.g., `2h`, `45m`, `90s`). Must be greater than zero.
  Default is `30m`.
- `--no-review`: skip the preflight review workflow (review runs by default).
- `--no-commit`: skip the commit handoff (commit runs by default).
- `--no-project-prompt`: skip inclusion of the project instructions file for
  this single run.
- `--dry-run`: previews the selected executable, arguments, prompt, model,
  task ID, agent, requested orchestration flags, and (when review is enabled)
  review agent and model, without rewriting the task or invoking the external
  CLI.

Repository configuration:

```json
{
  "default_work_agent": "codex",
  "taskWork": {
    "promptFile": ".agents/prompt.md",
    "implementation": {
      "agent": "codex",
      "model": "gpt-5-codex"
    },
    "review": {
      "agent": "claude",
      "model": "sonnet"
    }
  }
}
```

Examples:

```bash
ahm task work 001
ahm task work 001 --agent codex
ahm task work 001 --agent cursor --no-review
ahm task work 001 --agent claude --no-commit
ahm task work 001 --timeout 2h
ahm task work 001 --model o4-mini
ahm task work 001 --agent codex --model o3-mini
ahm task work 001 --no-review
ahm task work 001 --no-commit
ahm task work 001 --no-project-prompt
ahm --dry-run task work 001 --agent cursor
ahm --dry-run task work 001 --model claude-sonnet-4
```

### `task complete <id>`

Sets a task status to `Completed`, moves it to
the completed task bucket, removes the old file when the bucket changed, and
regenerates indexes.

Before completing, `ahm` verifies that all task dependencies (listed in
`depends_on`) are already in `Completed` status. If any dependency is not
completed, the command returns an error listing the incomplete dependencies
and does not modify the task file or indexes.

After completing a task, `ahm` scans active `Blocked` tasks that directly depend
on the completed task. Any dependent task whose full dependency list is now
completed is changed to `Pending`, stamped with `updated`, and included in the
same index regeneration. Tasks blocked for unrelated reasons, or tasks that
still have incomplete dependencies, stay `Blocked`.

Before moving the task, `ahm` also checks for an acceptance section. It accepts
`##` or `###` headings named `Acceptance Notes`, `Acceptance Criteria`, or
`Acceptance`, case-insensitively. Completion prints stderr warnings when the
section is missing, still contains the seeded `- [ ] TODO` placeholder, or has
unchecked `- [ ]` or `* [ ]` checklist items.

By default, incomplete acceptance notes warn but do not block completion. Set
`"strict_acceptance": true` in `.ahm/config.json` after migration, or legacy
`.agents/ahm.json` before migration, to make those findings return
a non-zero error. The global `--force` flag overrides strict acceptance and
completes the task while still printing the warnings.

Before writing the completed task, `ahm` also inspects the task's `exec_plan`
field. If it resolves to a file in the active ExecPlan bucket, `ahm` prints a
stderr warning that includes the plan path and directs you to move the plan
to the completed ExecPlan bucket and update the task's `exec_plan` field.
If the active ExecPlan already has a filled Outcomes & Retrospective section,
the warning notes that the plan is ready to be completed. The warning does
not block completion — use `ahm status` for the full validation report after
the fact, or move the ExecPlan first and update the `exec_plan` field before
completing the task to avoid the warning entirely.

Alias:

- `task close <id>`

Useful flags:

- `--dry-run`: previews the target path, status, and any dependent tasks that
  would be unblocked without writing.
- `--force`: overrides `"strict_acceptance": true` for this completion.

Example:

```bash
ahm task complete 001
```

### `task comment <id> <text>`

Appends a lightweight timestamped comment to a task's `## Comments` section
and regenerates indexes.

Comments are stored in the task Markdown body and appear on all task outputs
including `task show`. Each comment is timestamped with the current time in
RFC 3339 format. Author metadata is omitted by default and included only when
`--author` is provided.

The command works on active, completed, and cancelled tasks. The task's
`updated` timestamp is set to the comment time and generated indexes are
regenerated to reflect the change.

```markdown
## Comments

**2026-06-24T18:30:00Z** — Found the root cause.

**2026-06-24T18:31:00Z** — _Author Name_: Follow-up observation.
```

All existing front matter, body sections, unknown fields, and other comments
are preserved. The new comment is appended after existing comments in the
section.

If the task has no `## Comments` section yet, one is created at the end of
 the body.

Useful flags:

- `--author <name>`: include an author name in the comment.
- `--body-file <path>`: read comment text from a file, or `-` for stdin.
  Mutually exclusive with positional text.
- `--dry-run`: previews the comment metadata without writing.
- `--json`: emits a structured comment record with `id`, `path`, `text`,
  and optional `author`.

Examples:

```bash
ahm task comment 116 "Found the root cause"
ahm task comment 116 --author "Travis" "Need to revisit this"
ahm task comment 116 --body-file -
ahm --json task comment 001 "Observation"
ahm --dry-run task comment 001 "Preview"
```

### `task cancel <id> --reason <text>`

Sets a task status to `Cancelled`, moves it to
the cancelled task bucket, removes the old file when the bucket changed, stores
the supplied reason in a `## Cancellation Reason` body section, and regenerates
indexes. The reason is required after trimming whitespace. The global `--force`
flag does not bypass this requirement.

When the task's acceptance notes still contain the seeded `- [ ] TODO`
placeholder, cancellation prints a warning but still proceeds.

Useful flags:

- `--reason <text>`: required reason for cancelling the task.
- `--dry-run`: previews the target path, status, and reason without writing.

Example:

```bash
ahm task cancel 001 --reason "Superseded by 002"
```

### `task reopen <id>`

Sets a task status to `Pending`, moves it to
the active task bucket, removes the old file when the bucket changed, and
regenerates indexes.

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Example:

```bash
ahm task reopen 001
```

### `task dep add <id> <dependency-id>`

Adds a dependency to a task, rewrites the task file, and regenerates indexes.

Both IDs use normal task resolution. Dependencies are stored by canonical task
ID, deduplicated, and sorted by task ID.

The command rejects unsatisfiable dependencies:

- **Self-dependency**: the operation fails with an error if the task ID and
  dependency ID are the same.
- **Cancelled dependency**: the operation fails with an error if the dependency
  task has status `Cancelled`, because a cancelled task will never be completed.
- **Cycle creation**: the operation fails with an error if the new edge would
  introduce a dependency cycle among non-completed, non-cancelled tasks.

Useful flags:

- `--dry-run`: previews the resulting dependency list without writing.

Example:

```bash
ahm task dep add 002 001
```

### `task dep remove <id> <dependency-id>`

Removes a dependency from a task, rewrites the task file, and regenerates
indexes.

Alias:

- `task dep rm <id> <dependency-id>`

Useful flags:

- `--dry-run`: previews the resulting dependency list without writing.

Example:

```bash
ahm task dep remove 002 001
```

### `task dep tree <id>`

Prints a dependency tree rooted at a task.

Missing dependencies are printed as `[missing]`. Cycles are detected during tree
walking and printed as `cycle to <id>`.

Example:

```bash
ahm task dep tree 002
```

### `task dep cycles`

Prints active dependency cycles.

Tasks with status `Completed` or `Cancelled` are excluded from cycle detection.
When no cycles are found, the command prints `No dependency cycles found`.

Example:

```bash
ahm task dep cycles
```
