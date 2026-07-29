# External Store For Workflow Records (XDG)

Status: active
Created: 2026-07-29
Updated: 2026-07-29
Related tasks: 255, 256, 257, 258
Related plans: -
Confidence: medium

## Summary

Move tasks, research notes, and ExecPlans out of the project tree entirely into
a per-project directory under an XDG data path such as
`$XDG_DATA_HOME/ahm/<project-key>/`. `ahm prime` tells the agent where that
directory is. The location is overridable per project, so a user who prefers
committed in-repo records can keep them under `.ahm/`.

This is Option B from [[records-storage-via-git-refs]], which that note rejected.
The rejection reasons were: no stable project key, poor discoverability, a break
with root-scoped writes (ADR 001), and no sync answer. Three of the four no
longer hold under the current framing, and the fourth has been explicitly
accepted as out of scope.

The decisive difference from the private-ref design is that **an external store
has no synchronization layer at all**. Every one of the six release blockers
that killed ADR 013 (recorded in ADR 015) was a synchronization failure —
divergence, worktree publishing, clone bootstrap, remote seeding, silent
snapshot loss, config dirtying. A plain directory has none of them by
construction rather than by repair. `ahm` does not become a storage engine; it
becomes a program that reads and writes files in a different directory.

### What Moves And What Does Not

ADR 015 drew the ownership line carefully; this design moves one bucket across
it and leaves the rest alone.

**Moves to the store:** task records, research notes and their attachments, and
ExecPlans — everything currently under `.ahm/tasks/`, `.ahm/research/`, and
`.ahm/exec-plans/`, plus the generated workflow indexes and the record lock.

**Stays committed in the project:** ADRs under `docs/adr/` and the
project-facing `docs/adr/index.md`; `.agents/` project-owned agent content;
`AGENTS.md`; and `.ahm/config.json`, which shrinks to a repository-scoped
anchor carrying the project key.

The graduation path is unchanged and is what makes the split coherent: durable
outcomes leave the store and land in committed ADRs and design docs.

## Why This Is Viable Now

### The motivation is git coupling, not repo hygiene

The stated pains are commit/PR noise, cross-branch and worktree friction, and
merge conflicts on records. Repo hygiene — "a tool's state doesn't belong in my
tree" — was explicitly *not* a motivation.

That matters more than it appears. All three named pains are symptoms of one
cause: records are git-tracked, so they inherit branch scoping, merge semantics,
and diff visibility. An external store removes the coupling completely rather
than partially, and it addresses all three at once. It also means the design is
free to keep a small committed anchor file in the repo, which resolves the
project-key problem at no cost against the actual goals.

### The problem is measurably large in this repository

Measured on `ahm` itself, 2026-07-29, across 420 non-merge commits:

| Measure | Value |
| --- | --- |
| Commits that exist only to move workflow records | 88 (21%) |
| Commits touching records at all | 334 (80%) |
| Share of changed lines in records, last 100 commits | 27% |

One commit in five exists solely to advance workflow state, and four in five
carry record churn alongside code. This is the strongest available evidence for
the stated motivation, and it also bounds the upside: the design removes about a
fifth of this repository's commits and roughly a quarter of its diff volume from
project history.

Note that the 21% records-only figure means the "commit records separately"
convention is already partly in use here, and PR noise persists regardless —
which weakens the cheapest alternative to this design.

### Path resolution is already centralized

`workflowPaths` (`internal/ahm/workflow_paths.go:14`) is the single struct that
resolves record locations, reached through one cached accessor
(`internal/ahm/cli.go:68`). Adding an external layout is a third branch in that
struct plus a store-resolution step, not a scatter-gather edit across the
command surface. This is materially cheaper than the ref design, which had to
touch migration, prime, config, validation, and a new sync state machine.

### It deletes machinery rather than adding it

Nothing in the store is git-tracked, so the apparatus ADR 015 built to keep
derived artifacts out of history stops being necessary. The managed
`.ahm/.gitignore`, the rule that generated indexes are ignored machine-local
derivatives, the care taken to keep committed configuration from being dirtied
by routine commands, and the distinction between committed `docs/adr/index.md`
and ignored workflow indexes all collapse into "the store is not in git."

This is worth weighing against the new surface the design introduces
(store resolution, export/import, a link scheme). Some of the added complexity
is offset by removed complexity, which is not true of most storage changes.

### The sandbox objection is solvable, and it was the sharpest one

The default `task work` agent is `cake` (`internal/ahm/task_agents.go:115`),
launched with `cmd.Dir = root` (`internal/ahm/task_work.go:274`) and — unlike
`claude` and `codex` — no sandbox-bypass flag. Cake's sandbox has already
blocked out-of-tree access in this repo (see [[cake-sandbox-blocks-worktree-git]]
for the worktree `.git` case). An XDG store is out-of-tree by definition, so the
concern was not just agents reading record files directly: the `ahm` child
process the delegated agent spawns runs *inside* that sandbox, so `ahm task
start`, `ahm task comment`, and `ahm task complete` would all write to a blocked
path.

Cake supports this directly. From `~/Projects/cake/docs/security.md:21`:

> `--add-dir` adds a read-only path for one invocation. `directories` in
> settings adds persistent read-write paths.

and `docs/configuration.md:60`:

> Top-level and profile `directories` grant persistent read-write access to the
> listed directories under `workspace-write`. Global and project entries are
> merged.

The implementation confirms it: `valid_settings_dirs`
(`~/Projects/cake/src/main.rs:560`) feeds `settings_dirs` into the writable set
at `src/clients/tools/sandbox/mod.rs:134`, alongside cwd, linked-worktree git
dirs, and toolchain caches. `--add-dir` would not have worked — read-only,
single invocation.

The grant can be **project-scoped rather than global**. Cake loads project
configuration from `{project_dir}/.cake/settings.toml`
(`src/config/settings.rs:403`), and `directories` entries from global and
project files are *merged* rather than overridden
(`~/Projects/cake/docs/configuration.md:17`). Because `ahm task work` sets
`cmd.Dir = root`, project settings resolve from the repository root, so a
committed `.cake/settings.toml` is picked up by delegated runs.

Cake dogfoods exactly this pattern for its own external state.
`~/Projects/cake/.cake/settings.toml` reads:

```toml
directories = [
  "/Users/travisennis/.cake",
  "/Users/travisennis/.cache/cake",
  "/Users/travisennis/.local/share/cake",
  "/Users/travisennis/.config/cake",
]
```

The `ahm` equivalent is a project `.cake/settings.toml` granting write access to
exactly that project's store directory. That this is how cake solves its own
version of the problem is reasonable evidence the mechanism is intended for this
use rather than being a workaround.

**Caveat: `directories` values are literal paths with no expansion.** Cake has
no `shellexpand` dependency, and `dirs::home_dir()` is used only for cake's own
internal paths — never to expand user-supplied settings values. Relative values
resolve from the invocation working directory
(`~/Projects/cake/docs/configuration.md:56`), which cannot express a home-anchored
store path stably. So a committed grant hardcodes one user's home directory, as
cake's own example does. This is fine for a solo developer and does not
generalize to multiple users or differing machine layouts.

Cake task 277 (Pending) already covers this: it adds `~` expansion to
`directories` and a structured `[sandbox]` section, with the explicit acceptance
criterion that `directories = ["~/foo"]` expands and works. No `ahm`-side work
depends on it landing, but a portable committed grant does.

**Ordering hazard: a grant for a directory that does not exist is dropped.**
`valid_settings_dirs` (`~/Projects/cake/src/main.rs:560`) filters entries on
`p.exists() && p.is_dir()` and discards the rest with only a `tracing::warn!`.
If `ahm` creates a project's store lazily on first write, and a delegated agent
launches before that first write, the grant evaporates and the write is denied —
with no signal at configuration time. **This constrains the `ahm` side
regardless of what cake does: the store directory must be created eagerly, at
`init` or migration time, not on first use.** Filed as cake task 323, which
depends on 277.

## Decisions Settled

| Question | Decision |
| --- | --- |
| Branch scoping | Reversed deliberately. The backlog becomes repo-global and branch-agnostic. |
| Durability | The user's own machine backup. `ahm` provides no sync and says nothing. |
| Machine moves | Plain `ahm store export` / `import`: explicit, manual, offline. |
| Cross-references | A resolvable link scheme, validated by `ahm`, in both directions. |
| Undo | None. Accepted as a cost. |
| Layout scope | External becomes the default; in-repo `.ahm/` is the override. |
| Project key | Explicit committed ID (implied by the above; see below). |

### Export/import, not sync

A fresh clone or a second machine finds a committed project key whose store does
not exist locally, so the backlog starts empty and stays empty. Today a clone
carries the whole backlog, so this is a genuine regression, and it is the
fresh-clone problem that damaged the ref design's usability — reappearing here
as intended behavior rather than as a bug.

The answer is `ahm store export` and `ahm store import`: explicit, manual,
offline commands with no daemon, no conflict resolution, and no network. They
cover machine moves, backups, and handing a backlog to someone else without
reintroducing the synchronization engine that ADR 015 rejected.

The boundary to hold: import is a user-invoked operation with a defined
collision policy, not a background reconcile. Two machines that both edit
between exports will produce a collision the user resolves, and `ahm` should
not pretend otherwise. Whether import refuses on any collision, merges
non-overlapping records, or writes both sides for manual resolution is an open
design question.

### Branch scoping is reversed on purpose

ADR 015 made branch-scoped records an explicit, accepted property. This design
reverses it. Gains: tasks survive branch switches, all worktrees of a repo share
one backlog, and sequential-ID add/add conflicts at merge time disappear
entirely. Costs, accepted: there is no per-commit history of what the backlog
looked like, a task record no longer travels with its PR, and reviewers cannot
see the record that specified the work.

### The project key must be an explicit committed ID

Absolute path breaks when a directory is moved or renamed, and splits clones and
worktrees into separate backlogs. Remote URL breaks on no-remote, multi-remote,
ssh-versus-https forms, and forks. The only key stable across all of those is an
explicit ID written once into a committed file — the natural home is a
`project_id` field in `.ahm/config.json`, which `detectManagedRoot`
(`internal/ahm/root.go:36`) already anchors on.

Consequence worth stating plainly: **"records leave the project" does not mean
"`.ahm/` leaves the project."** A small committed anchor remains. Against the
actual motivations this costs nothing — the anchor is written once, never
churns, never conflicts, and never appears in a PR diff after creation.

### ADR 019 downgrades from blocker to strongly desirable

There is no `ahm research` or `ahm plan` command; ADR 019 is still
`status: proposed`. `ahm context research` instructs agents to write files
directly into `.ahm/research/`. Under an external store those become absolute
out-of-tree writes — which the cake `directories` grant permits. So ADR 019 is
not gating. It remains strongly desirable: routing record writes through the CLI
would let agent-facing output stop naming filesystem paths at all, which is the
cleaner contract and reduces dependence on every agent's sandbox configuration.

## Open Problems

### Decision-gating

1. **Task 218 collides with this design.** Task 218 ("Enforce read-only
   permissions for advisory agent delegations") is Pending. Cake's `read-only`
   policy demotes `directories` back to read-only
   (`~/Projects/cake/docs/security.md:15`). A review agent running read-only
   could not run `ahm task comment`. Either advisory delegations keep a write
   channel for records, or record writes must be performed by the parent `ahm`
   process rather than the delegated agent. This needs resolving before either
   design lands.

2. **Default-flip migration must not orphan existing backlogs.** "External is
   the default" can only apply to new installs. Any repository that already has
   committed records under `.ahm/tasks/` must be treated as having an implicit
   in-repo override, and keep working untouched until explicitly migrated.
   Without that, an `ahm` upgrade silently points live repositories — including
   this one, with 20+ ready tasks, 8 blocked, and a completed archive — at an
   empty store. This is the single sharpest operational risk.

3. **Migration must rewrite existing cross-references or knowingly break them.**
   The resolvable-link scheme fixes the future. It does not fix the ADRs already
   committed: ADR 015 cites a research note by relative path, and
   `validateMarkdownLinks` (`internal/ahm/validation.go:774`) is what emits the
   `markdown_link_missing` warning visible in `ahm prime` today. Migration either
   rewrites those citations in committed docs, or accepts permanent dangling
   links in the project's decision history.

4. **ADR 008's commit handoff instructs the agent to commit task files.** The
   delegated commit handoff asks the agent to include "both task files and
   project source files in one commit"
   (`docs/adr/008-delegated-task-work-commit-handoff.md`), and consuming
   projects encode the same expectation — cake's `AGENTS.md` states that
   "task-state and index files are commit content." Under an external store
   task files are not commit content at all, so the prompt becomes wrong and
   partly impossible to satisfy. ADR 008 and the handoff prompt text both need
   revision, and any project instruction that mirrors the old expectation needs
   updating.

### Serious, probably mitigable

5. **`ahm` cannot guarantee another tool's sandbox config.** A project
   `.cake/settings.toml` is committed, so the grant travels with the repository
   rather than being purely user-scoped out-of-band setup — this is much better
   than a global grant. But because `directories` values are literal
   unexpanded paths, a committed grant encodes one home directory and breaks for
   a second user or a different machine layout. It is also cake-specific: every
   other sandboxed agent needs its own equivalent, and a future agent may offer
   no escape hatch at all. `ahm doctor` can detect and report a missing grant;
   it cannot fix one.

   Open boundary question: should `ahm` *write* `.cake/settings.toml`? That is
   another tool's project-owned config. The established pattern here is
   `ahm onboard`, which prints a snippet for the user to place rather than
   writing the file. The same approach likely fits — detect in `doctor`, emit
   the snippet on request.

6. ~~**The grant widens blast radius across projects.**~~ **Resolved by
   project-scoped grants.** A global `directories` entry would make every
   project's store writable from any project. A project `.cake/settings.toml`
   can name exactly one store directory instead, and merge semantics mean it
   composes with global settings without replacing them. The narrower grant
   costs nothing in complexity — it is one committed file per project.

7. **Test isolation becomes a live hazard for the first time.** The suite uses
   `t.TempDir()` in 388 places, and the codebase contains no reference to
   `XDG_DATA_HOME`, `UserHomeDir`, or `HOME` anywhere — every path derives from
   `root`, which is what structurally prevents a test from escaping its
   sandbox today. Store resolution introduces the first real home-directory
   dependency, so a bug in a mutating test could write to or destroy the
   developer's actual backlog. Store resolution must be environment-overridable,
   every test must set the override, and something should fail loudly if a test
   ever resolves a store outside its temp directory.

8. **Migration rollback has two live copies and no defined reconciliation.**
   After migrating out, `git revert` restores the in-repo records, but the
   store still holds its own copy which has since diverged. "Reversible,
   previewed path" is asserted below without saying which copy wins, whether
   rollback exports the store back into the tree first, or how a user who has
   worked for a week post-migration gets a coherent single history. This needs
   specifying alongside export/import, which is the natural mechanism for it.

9. **The record lock must move into the store, and a stale lock gets a wider
   blast radius.** `withWorkflowRecordLock` (`internal/ahm/lock.go:26`) is
   repository-scoped under `.ahm/.lock/`; it has to become store-scoped, which
   is mechanical but required.

   Contention itself is not the concern. A worktree works one accepted task at
   a time, so parallel checkouts mutate *different* records with short writes
   that serialize cleanly under a 10s acquire timeout and 10ms retry. The
   shared lock is a net improvement on the one case that did collide:
   ADR 010's ID allocation stops producing add/add conflicts at merge and
   simply hands out distinct IDs.

   What genuinely worsens is stale-lock impact. `workflowLockStaleAfter` is 30
   minutes with a 5-minute heartbeat, so a process that dies holding the lock
   currently blocks one checkout and would then block *every* checkout of that
   project until the staleness window expires. Bounded and rare, but worth
   revisiting the tuning for a lock shared by unrelated worktrees.

10. **Orphaned stores accumulate.** Deleting or archiving a project leaves its
    store directory forever, with no back-reference to prove it is dead. Needs
    `ahm store list` / `ahm store prune`, or at minimum a documented location.

11. **Two clones of one repo now share a backlog.** This follows from the
    committed project ID and is consistent with "repo-global," but it means a
    throwaway or experimental clone mutates the real backlog rather than getting
    an isolated copy. Worth an explicit escape hatch.

### Accepted costs

12. **No operational undo.** Outside git, a botched `ahm task groom` across many
    records has no `git revert`. Accepted.

13. **No durability without user backup.** This is a regression against today,
    where committed records survive machine loss for free. It is also arguably
    worse than gitignored-in-repo, since `~/Projects` tends to fall inside
    people's backup scope while `~/.local/share` tends not to. Accepted, with
    `ahm store export` as the deliberate manual mitigation.

14. **Records become invisible to code review and to anyone without `ahm`.**
    Accepted; consistent with the agent-first framing already established in
    [[records-storage-via-git-refs]].

15. **Records leave the repository's secret-scanning and ignore scope.** A
    credential pasted into a task body currently sits in a tracked file that
    pre-commit hooks and host-side scanning can catch. In the store it sits in
    an unscanned directory. Low severity given the agent-first, solo-first
    target, but it is a real reduction in an existing safety net.

## Implications For This Project

- The root-scoped-write posture needs amendment: `ahm` would write outside the
  detected repository root as routine behavior. The authoritative statement is
  `docs/guardrails/safety-and-permissions.md:35`, which lists "following a path
  outside the target repository without explicit intent" as a failure mode —
  *not* ADR 001, which covers only atomic writes and concurrency. The
  `ARCHITECTURE.md` System Boundaries section also needs updating. This is a
  compatibility-surface change and needs an ADR that supersedes or amends
  ADR 015.
- `workflowPaths` grows a third layout plus store resolution from a project key.
  Validation, migration, `records doctor`, templates, docs, and the test matrix
  all widen.
- The link scheme touches `validateMarkdownLinks`, the `markdown_link_missing`
  finding code, every record template, and the `ahm context *` guidance text.
- A new `ahm store` command surface is implied: `path`, `export`, `import`, plus
  `list` and `prune` for problem 10.
- `ahm doctor` gains a sandbox-grant check (problem 5) and a store-reachability
  check.
- ADR 008 and the delegated commit-handoff prompt need revision (problem 4), as
  does any consuming project's `AGENTS.md` that mirrors the old expectation.
- The record lock moves into the store; stale-lock tuning is re-examined for
  cross-checkout blast radius (problem 9).
- Test infrastructure needs a mandatory store-root override before any store
  resolution code lands (problem 7).
- Unlike ADR 013, there is a live adopter — this repository — so migration
  cannot be a direct replacement. It needs a real, reversible, previewed path.

## Readiness To Plan

This note is not yet a basis for tasks. `AGENTS.md` routes a
compatibility-surface change through an ADR, and cross-cutting work through an
ExecPlan, so the sequence is research → settle the decision-gating items → ADR →
ExecPlan → tasks. Two gates sit between this note and a task list.

Before an ADR can be drafted, four things are missing beyond the decision-gating
problems above:

1. **The cake grant has never been run.** The design's viability rests on an
   inference from cake's source and docs, not an execution. This is a cheap
   experiment and it should precede any commitment, because a negative result
   changes the design fundamentally rather than marginally.
2. **The cheap alternatives are unpriced.** An ADR needs considered options.
   Committing records in separate commits addresses PR noise; a `.gitattributes`
   union merge driver addresses record conflicts. Neither addresses cross-branch
   or worktree friction, which only relocation fixes. The measurements above
   suggest the separate-commit convention is already partly adopted and has not
   solved the problem, but that argument should be made explicitly rather than
   assumed.
3. **Four specifications determine task decomposition** and cannot be deferred
   into implementation: the store path and project-key scheme including how
   existing repositories acquire a key; the concrete link syntax; the
   export/import format and collision policy; and what `ahm prime` emits when
   records are external.
4. **Exit criteria are undefined.** ADR 013 was abandoned cheaply because it had
   no adopters. This design makes `ahm` itself a live adopter on day one, so the
   decision to back out will be expensive by comparison. How reversal works, and
   what evidence would trigger it, belongs in the ADR rather than discovered
   afterward.

Repository context that bounds migration risk: `ahm` is public but has no forks
and no stargazers, so a default-flip has no known external adopters to strand.
The risk is concentrated in this repository's own backlog.

### Work that does not depend on this decision

Some slices are independently valuable and were tasked on 2026-07-29. Each
lands value whether or not the store is adopted, and de-risks it if it is:

- **Task 255** — ADR 019's research and plan lifecycle commands. Removes agent
  dependence on record paths, desirable on its own and reduces this design's
  exposure to every agent's sandbox configuration.
- **Task 256** — `ahm store export` / `import`. Useful as backup and
  machine-transfer today against the committed layout, and the mechanism this
  design needs for both durability and migration rollback.
- **Task 257** — test path-escape guard. Prerequisite here, harmless hardening
  otherwise; sequenced to land before any store resolution exists.
- **Task 258** — the resolvable link scheme. Makes record references robust
  against lifecycle moves today and against relocation later.

None of these commits the project to the external store. If this note goes no
further, all four still stand on their own.

## Follow-ups

- Resolve the task 218 conflict before committing to either design.
- Specify the implicit-override rule for repositories with existing committed
  records, and prove it with a test that an upgrade never orphans a backlog.
- Empirically verify the cake `directories` grant end to end: add a project
  `.cake/settings.toml` naming the store, run `ahm task work` against a
  store-backed repo, and confirm `ahm task start`, `ahm task comment`, and
  `ahm task complete` all succeed from inside the delegated agent. The reasoning
  above is from source and docs, not a live run.
- Decide whether `ahm` emits the `.cake/settings.toml` grant as an
  `onboard`-style snippet, checks for it in `doctor`, or both — and whether
  agent-specific grant guidance belongs in `ahm` at all.
- Design the resolvable link scheme concretely, including what happens to the
  existing ADR citations at migration time.
- Specify `ahm store export` / `import`: archive format, what is included, and
  the collision policy on import. Rollback (problem 9) should reuse it.
- Decide the store lock's location and whether the 30-minute staleness window
  is still right once a dead process can block every checkout of a project.
- Add the test store-root override and a guard that fails any test resolving a
  store outside its temp directory. This should land before, not with, store
  resolution.
- Decide whether ADR 019 should land first so agent-facing output can stop
  naming record paths entirely.
- Draft the ADR only after the task 218 conflict and the migration rule are
  settled.
