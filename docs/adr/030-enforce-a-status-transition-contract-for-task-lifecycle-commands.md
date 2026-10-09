---
status: accepted
date: 2026-10-09
decision-makers: Travis Ennis
informed: task 275
---
# Enforce a status transition contract for task lifecycle commands

## Context and Problem Statement

The lifecycle verbs `accept`, `start`, `complete`, `cancel`, `reopen`, `block`,
and `unblock` each move a task to a fixed target status, but the implementation
did not constrain which start states a verb accepts. The status path in
`internal/ahm/task_status.go` applied one guard: when the task already held the
verb's target status and bucket it printed `already X` and returned success, and
every other start state was transitioned unconditionally. Three consequences
followed:

- A verb invoked in a state it does not apply to silently succeeded or
  performed an unintended move. `task reopen` on a `Pending` task reported
  `already Pending`; `task start` on a `Completed` task moved it back to
  `active`; `task cancel` had no guard and could cancel a `Completed` task.
- Automation could not distinguish "this was already true" from "this
  invocation was wrong for the task", because both exited 0.
- The documented preconditions in each verb's `long` help and in the command
  reference did not match the executable contract.

The affected behavior — command semantics, exit codes, and help text — is a
compatibility surface, so changing it is a breaking change.

## Decision Drivers

- A silent success must not hide an unintended transition.
- Idempotent retry must stay safe: re-running the same command should not fail.
- The executable contract must match the documented one in one place.

## Considered Options

- Tolerant no-op everywhere (status quo): exit 0 for every inapplicable
  invocation, with no distinction between a retry and a wrong command.
- Usage error everywhere: reject any invocation whose start state is unlisted,
  including a retry that targets the status the task already holds.
- Target-status no-op with a per-verb accepted set (chosen).

## Decision Outcome

Chosen option: "Target-status no-op with a per-verb accepted set", because it
separates the two cases a caller cares about — a safe retry and a wrong
invocation — while staying small enough to state in one table.

Each verb declares the statuses it accepts and the status it targets. When the
resolved task already holds the target status in its expected bucket, the
command prints `<id> already <status>` and exits 0 without writing: the
command's desired end state already holds. When the task holds the target status
in the wrong bucket, the command repairs the placement. From any other start
state the command is a usage error (exit 2) whose message names the current
status and the accepted ones.

| verb | accepts from | target |
| ---- | ------------ | ------ |
| `accept` | Open | Pending |
| `start` | Pending | In Progress |
| `complete` | Open, Pending, In Progress, Blocked, Tracking | Completed |
| `cancel` | Open, Pending, In Progress, Blocked, Tracking | Cancelled |
| `reopen` | Completed, Cancelled, Pending | Open |
| `block` | Open, Pending, Tracking | Blocked |
| `unblock` | Blocked | Pending |

Two changes follow from the table. `reopen` targets `Open` instead of `Pending`:
reopening returns a task to the untriaged queue, so `accept` is required to
queue it again, and `reopen` is the inverse of `accept` for a `Pending` task.
`block` and `unblock` no longer rewrite a task that already holds their target
status; to correct a `Blocked` task's reason, release it with `unblock` and
block it again.

`Tracking` is a tracker status, not a work status: a tracker is created with
`task create --status Tracking` and is listed in the ready queue once its
children resolve. It is accepted by `complete` (close the tracker), `cancel`
(abandon it), and `block` (pause it); `accept`, `start`, `reopen`, and `unblock`
reject it. Omitting `Tracking` from `complete` would leave a tracker
uncloseable, because no other command sets a task's status.

### Consequences

- Good, because a wrong invocation fails loudly with a non-zero exit and a
  message that names the current and accepted statuses.
- Good, because an idempotent retry (`start` on `In Progress`, `complete` on
  `Completed`) still succeeds.
- Good, because the help text and the command reference can state one contract
  the code enforces.
- Bad, because four previously tolerated or unguarded invocations now exit 2:
  `accept` on a non-Open task, `start` on a non-Pending task (including
  restarting a `Completed` task), `cancel` on a `Completed` task, and `block`
  on an `In Progress` task.
- Bad, because `reopen` no longer returns a `Completed` task to the ready queue
  in one step, re-blocking no longer corrects a recorded reason, and cancelling
  an already `Cancelled` task discards a newly supplied reason instead of
  rewriting it.

## More Information

- Task 275.
- Applies to the transition behavior previously described in
  `docs/references/workflow-spec.md` and
  `docs/references/cli/task-commands.md`.
- Partially supersedes ADR 028: `task block` accepts only `Open` and `Pending`
  tasks and no longer rewrites a recorded reason, replacing that ADR's "any
  non-terminal status" clause. `task unblock` still accepts only a `Blocked`
  task, but invoking it on a `Pending` task is now a no-op rather than an error.
- Related: ADR 007 (task cancellation reasons).
