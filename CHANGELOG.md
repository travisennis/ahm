# Changelog

All notable user-facing changes to `ahm` are recorded here.

## v2.0.0 - 2026-09-26

### Breaking Changes

v2.0.0 reduces `ahm` to a records CLI for tasks and ADRs. The full release
notes, which are the body of the GitHub Release, are
[docs/releases/v2.0.0.md](docs/releases/v2.0.0.md).

- **Removed commands:** `ahm context`, `ahm agents`, `ahm task work`,
  `ahm upgrade`, `ahm task migrate`, and `ahm adr migrate` all shipped in v1;
  `ahm audit`, `ahm onboard`, `ahm task groom`, the `ahm docs` group, and the
  `ahm records` group were removed before they ever shipped in a release.
  There are no aliases and no replacements. Delete every hook, CI step,
  script, and instruction that calls one.
- **Removed record families:** research notes (`.ahm/research/`) and ExecPlans
  (`.ahm/exec-plans/`). `ahm` no longer reads, indexes, or validates them.
  Nothing is deleted. The task front-matter field `exec_plan` is retired.
- **Removed configuration keys:** `taskWork`, `default_work_agent`,
  `projectDocs`, and `research`. `ahm init` drops them from
  `.ahm/config.json` and preserves every other field.
- **Upgrade step:** run `ahm init`. A repository still on the legacy
  `.agents/ahm.json` layout is refused by every command and has to be moved
  with [v1.1.0](docs/releases/v1.1.0.md) first; see
  [the workflow upgrade guide](docs/guides/workflow-upgrades.md).
- **Changed storage location:** task records can move to a user-level home
  store. Existing repositories keep project-local records until they opt in
  with `ahm store migrate --to home`; a repository with no
  `.ahm/config.json` defaults to the home store. `tasks_location` in
  `.ahm/config.json` and the absolute `AHM_HOME` environment variable select
  the store.

### Added

- *(store)* Derive project identity and resolve the home store root
- *(tasks)* Read and write task records in the home store
- *(store)* Persist a non-decrementing task ID counter
- *(store)* Add ahm store migrate for task record location
- *(store)* Default new projects to the home store

### Fixed

- *(test)* Assert the legacy layout path with native separators
- *(cli)* Stop index dry-run from deleting stale temp files
- *(release)* Make git-cliff output pass markdownlint
- *(cli)* Keep structured record paths slash-separated on Windows

### Changed

- *(cli)* Remove task work, audit, and groom
- *(workflow)* Retire research and ExecPlans as record families
- *(cli)* Remove the procedure channel and prime's prescriptions
- *(cli)* Collapse install and upgrade into one idempotent ahm init
- *(core)* Split workflow paths into project and record roots

### Documentation

- *(plan)* Record milestone 5's CI result and Windows separator finding
- *(workflow)* Rewrite documentation and instructions to the records boundary
- *(adr)* Record the plan for the home store for task records
- *(workflow)* Document the home store boundary
- *(release)* Publish per-tag release notes and add v2.0.0 notes
- *(release)* Add the v1.1.0 release notes

### Maintenance

- *(workflow)* Dispose of the backlog made moot by ADR 022
- *(workflow)* File follow-up tasks for the store task ID counter
- *(workflow)* Mark 267e complete
- *(workflow)* Migrate task records to home store

## v1.1.0 - 2026-09-20

### Added

- *(workflow)* Add records metadata model
- *(workflow)* Add ref-backed records plumbing
- *(cli)* Add records sync commands
- *(cli)* Add records migrate command
- *(workflow)* Integrate ref-backed records with workflow write paths
- *(cli)* Make task work agent timeout configurable via --timeout flag
- *(prime)* Add ahm prime session briefing command, deprecate unscoped context
- *(prime)* Add ahm prime records sync and stale record reporting
- Use non-dot record directories under .ahm
- *(workflow)* Render agent guidance paths by storage mode
- *(task-work)* Add --model flag for agent model override
- *(task-work)* Configure agents and models by role
- Surface workflow validation warnings after record mutations
- *(task-complete)* Warn when completing tasks linked to active ExecPlans
- *(tasks)* Add delegated backlog grooming
- *(cli)* Add delegated improvement audit
- *(task-work)* Embed preflight review procedure
- *(cli)* Replace agent suggestions with onboard
- *(workflow)* Replace managed skills with procedures
- Decouple record layout from ref storage mode (task 172c)
- Rework records migrate to preserve normal Git tracking
- Retire ref-backed records commands and sync metadata
- *(prime)* Regenerate indexes and ensure .ahm/.gitignore on init and prime
- Add committed .ahm migration and topology regression coverage
- Add ahm docs check command and expanded static checks
- *(tasks)* Apply structured grooming revisions
- Support multiple task IDs in ahm task show
- *(adr)* Add ahm adr propose command with lifecycle transition enforcement
- *(init)* Make fresh ahm init create only the committed .ahm layout
- Configure task work agents and models
- *(workflow)* Stop installing scaffold readmes
- *(tasks)* Add configurable list sorting
- *(tasks)* Retry invalid grooming verdict batches
- *(research)* Surface stale inbox notes
- *(docs)* Bound project-doc discovery walk with depth limit and expanded skip list
- Detect duplicate task IDs across buckets in validation
- *(workflow)* Integrate research and documentation into task routing
- *(cli)* Remove general documentation surface
- *(context)* **breaking:** Remove the documentation workflow scope
- Add task 259 for --depends-on flag on ahm task create
- Add tasks for Node 20 CI action upgrade and Windows root canonicalization
- Add --depends-on flag to task create
- *(workflow)* Signal tracking tasks whose children are all complete

### Fixed

- *(tasks)* Avoid duplicate formatted task headings
- *(agent)* Require cake review sessions
- *(validation)* Ignore markdown links inside inline code spans
- *(tasks)* Constrain Cake delegated results
- *(groom)* Keep Comments last when groom adds new task sections
- *(tests)* Finish .ahm-first init follow-through missed by task 175
- *(core)* Scrub inherited Git repository environment
- *(cli)* Report init indexes for active layout
- *(workflow)* Finalize delegated tasks after review
- *(task-work)* Require completed orchestration
- *(workflow)* Preserve legacy procedure skills
- *(agent)* Strip ANTHROPIC_API_KEY from claude task work environment
- *(cli)* Pass --dangerously-skip-permissions to Claude Code during task work
- *(tasks)* Re-resolve task state inside status mutation lock
- *(groom)* Prevent dependency slice aliasing during task groom
- *(workflow)* Serialize all workflow record mutations with one lock
- *(tasks)* Make rendered front matter round-trip safe
- Protect active atomic temp files from stale cleanup
- *(internal/ahm)* Handle front matter closing delimiters at EOF
- *(workflow)* Detect asterisk checklist items in completed ExecPlan progress
- *(adr)* Preserve dates on idempotent status changes
- *(cli)* Sort recent research before prime cap
- *(cli)* Stabilize install result schema
- *(tasks)* Reject overflowing numeric IDs
- *(workflow)* Clarify atomic write path contract
- *(workflow)* Make lock reclamation ownership-safe
- *(tasks)* Reject work with unparseable dependencies
- *(workflow)* Modernize agent onboarding guidance
- Allow audits with an empty task label vocabulary
- Reject grooming of terminal and non-Open/Blocked task records
- Apply task and ADR migrations under --json and --plain
- *(indexes)* Warn when ADR parse errors are skipped during index generation
- *(prime)* Include full lifecycle steps in "Work a task" intake bullet
- *(workflow)* Prevent stale-lock reclamation from stealing live locks
- *(tasks)* Reject stale grooming verdicts
- *(validation)* Constrain managed link discovery
- Eliminate exponential dependency tree expansion in task dep tree
- *(adr)* Escape front matter scalars when rendering ADR records
- Make atomic writes and directory removals work on Windows
- Surface ADR collection errors and normalize Windows output paths
- Use owner tokens for workflow lock identity across platforms
- Avoid opening lock token files for fresh locks on Windows
- Clean up workflow lock cleanup and heartbeat lifecycle leaks
- *(adr)* End created ADR files with a single newline
- *(adr)* Keep a single trailing newline when rewriting ADRs

### Changed

- Remove automatic ref sync from prime and workflow mutations (172e)
- *(cli)* Remove record storage mode output
- *(workflow)* Memoize command path resolution
- *(workflow)* Consolidate legacy migration helpers
- *(workflow)* Consolidate markdown section helpers
- *(workflow)* Reuse parsed mutation state
- *(workflow)* Centralize markdown link walking
- Remove vestigial template version tracking
- Remove dead wrappers and duplicated helpers in internal/ahm
- *(workflow)* Reuse parsed records across index generation and validation

### Build

- Upgrade GitHub actions to Node 24 (#1)
- Run CI on all pushes and route release commits through a branch + PR (#3)
- Refuse direct commits to master via pre-commit guard hook (#5)

### Documentation

- *(workflow)* Accept ref-backed records ADR
- *(adr)* Record command-based procedure decision
- *(workflow)* Update ref-backed records guidance
- *(adr)* Revise ADR 014 to delegated procedures with mechanical apply
- *(prompt)* Repair truncated task briefing and update git-safety boundary to .ahm/
- *(workflow)* Fix workflow spec list indentation
- *(workflow)* Supersede ADR 013 and plan committed .ahm transition
- *(adr)* Propose ahm docs check and project-docs deprecation
- Update documentation and release guidance for committed .ahm records
- *(workflow)* Simplify task workflow reference
- Add review and oracle steps for operating flow
- *(tasks)* Complete task 216 with live Claude smoke evidence
- Update managed work with ahm section
- *(workflow)* Design plan and research lifecycle commands
- Align AGENTS.md operating loop with ahm task lifecycle
- Trim CLI refs to contract-only, delete stale artifacts, collapse module map
- Update AGENTS.md operating loop to match current workflow
- Annotate AGENTS.md routes and add agent-instructions guardrail
- *(adr)* Define structured work boundary
- Bound the operating-loop review cycle
- Fix fenced code block language specifiers in dep tree docs
- Research external store for workflow records
- Document worktree-based feature-branch workflow in CONTRIBUTING.md (#4)
- Adopt feature-branch workflow in AGENTS.md and CONTRIBUTING.md (#6)
- *(workflow)* Reconcile .agents/prompt.md with committed-record and feature-branch workflow (#7)
- *(adr)* Record the plan to reduce ahm to tasks and ADRs
- *(adr)* Supersede the nine ADRs ADR 022 replaces

### Tests

- *(tasks)* Cover formatted heading lifecycle rewrites
- Fix flaky blocked-deps assertion matching the temp-dir root

### Maintenance

- *(tasks)* Record task work session audit task
- *(tasks)* Capture workflow follow-up tasks
- Symlink .claude/skills to .agents/skills
- *(tasks)* Add task edit command backlog item
- Clean up workflow status warnings
- *(tasks)* Record workflow validation follow-ups
- *(tasks)* Plan committed ahm records migration
- *(tasks)* Add tasks 173 and 174
- Address task 172 release-review findings and close tracker
- *(tasks)* Plan structured grooming revisions
- Remove dead ref-mode remnants from records code
- *(tasks)* Record .ahm-first init decision and groom section-order bug
- *(tasks)* File GIT_DIR test-isolation and init output-path bugs (183, 184)
- *(tasks)* File worktree isolation spike for ahm task work (185)
- *(tasks)* Track delegated-work reliability gaps
- Upgrade ahm
- *(tasks)* Capture task work improvements
- *(tasks)* Capture review findings
- *(workflow)* Add project-owned agent skills
- *(tasks)* Groom backlog and add recovery task
- *(tasks)* Groom audit findings
- *(tasks)* Add configurable sorting task
- *(tasks)* Complete task 215 and record live smoke evidence
- *(tasks)* Accept task 216 for Claude Code permission denials
- *(tasks)* Add task 217 for stale research-inbox disposition
- Upgrade ahm
- *(tasks)* Record task 209 acceptance
- *(tasks)* Groom backlog and add audit findings
- *(tasks)* Record code-review findings as tasks
- *(tasks)* Groom backlog
- *(tasks)* Complete task 221, create task 233 for lifecycle guidance gaps
- *(tasks)* Add documentation audit findings
- *(tasks)* Plan research and docs routing
- *(tasks)* Narrow ahm documentation scope
- *(tasks)* File source review findings as tasks
- *(workflow)* Complete task 241, clear doctor warnings, enable strict acceptance
- *(workflow)* Plan feature-branch development adoption (tracker 263) (#2)
- *(workflow)* Enable GitHub branch protection on master (263b) (#8)
- *(workflow)* Commit directly to master instead of via pull requests

## v1.0.0 - 2026-07-03

### Added

- *(skills)* Add finding-improvements skill managed by ahm
- *(cli)* Add context command
- *(cli)* Improve agents suggestions integration guidance
- *(context)* Redesign ahm context role around managed-work intake
- *(cli)* **breaking:** Invert ahm task work defaults, replace --review/--commit with --no-review/--no-commit
- Add ahm task comment command (#116)
- *(tasks)* Add CLI improvement tasks from agent-focused review
- *(cli)* Add examples to --help for all commands
- Add JSON tags to Task struct for consistent field naming
- *(task)* Add --priority and --effort filters to task list
- *(cli)* Add scope descriptions to context --help (task 127)
- *(cli)* Add prompt field to task work dry-run preview
- *(task)* Add JSON output for task dep cycles and dep tree
- *(cli)* Add task search command for title matching
- *(adr)* Add ADR 012 superseding ADR 008, close out ExecPlan 074
- *(task)* Add subtask creation support to ahm task create
- *(task-work)* Include project instructions file in work prompts
- *(ci)* Add docs-md-lint to just ci

### Fixed

- *(cli)* Surface non-fatal parse errors as structured warnings instead of inline stderr (task 105)
- *(ci)* Scope local pre-commit hooks to pre-commit stage
- *(task-work)* Add 30-minute timeout to external agent execution
- Empty list output in text mode and JSON null vs empty array (task 122)
- *(cli)* Add example invocations to usage error messages
- *(cli)* Warn on context task parse failures
- *(workflow)* Keep cleaning stale temps past os.Remove failure
- *(workflow)* Replace os.Remove with os.RemoveAll in removeStaleWorkflowLock
- *(adr)* Serialize adr create ID allocation with workflow lock
- Reject newlines in task and ADR create fields rendered into front matter (closes #148)
- Fix slice aliasing that corrupts reported dependency cycles
- *(cli)* Drain and deduplicate warnings on emit to stop duplicate stderr output
- *(tasks)* Serialize task status transitions to close dependent auto-unblock race

### Changed

- *(validation)* Scope exec-plan sections cache to validationReport
- *(cleanupStaleTemps)* Collapse duplicate branches into single guard (task 106)
- *(task_commands)* Split god file into focused files (task 107)
- Move relPath from agents.go to path.go
- *(agents)* Align suggestions output with project AGENTS.md style
- *(output)* Add textRenderer interface, reduce any dispatch in text rendering
- *(templates)* Shrink DOCS.md to repo-specific workflow

### Build

- Skip pre-commit Go checks when no .go files staged

### Documentation

- *(release)* Clarify repeatable release checklist
- *(docs)* Route agents to CONTRIBUTING.md for command discovery
- *(cli)* Enumerate valid statuses in ahm task list --help (task 108)
- Add concept glossary mapping terms to implementing types
- *(workflow)* Document partial-write behavior of mid-batch index writes
- *(workflow)* Clarify ahm intake routing for agents
- Add Go module-path fallback for restricted agents
- Add tasks 118/119 to rework ahm task work flags
- *(preflight)* Require doc impact review
- *(cli)* Document task list filters
- Point contributors to docs context
- *(preflight)* Update the original workflow template for preflight
- *(cli)* Document root command exit code behavior in help (task 129)
- *(ops)* Add markdown linting, PR template, and doc lifecycle rules
- Require conventional commit messages
- Link workflow routing references
- Genericize task/ADR template label vocabulary and tighten TASKS.md
- Research agent instruction retrieval
- Research records storage via private git refs
- Plan ref-backed workflow record storage
- Add subtask creation support task
- Complete task 153 — add missing entries to ARCHITECTURE.md module map
- Fix broken adr/README.md links → adr/index.md
- Add vision doc and plan ahm docs check work
- Adopt .ahm/ as the tool-state namespace; add project-instructions records

### Tests

- Fix assertion patterns to surface all failures
- *(cli)* Add subprocess integration smoke tests
- *(lock)* Add tests for workflow lock mechanism (lock.go)

### Maintenance

- *(task)* Cancel 103 — Binary and templates.Version are independent
- *(tasks)* Add task 108 for enumerating valid statuses in help output
- Run ahm upgrade
- *(tasks)* Add 3 tasks from codebase audit (109-111)
- *(tasks)* Add 3 follow-up tasks from audit (112-114)
- *(tasks)* Groom backlog — fix labels, resolve decisions, accept all 6
- *(tasks)* Add context and comment tasks
- Run ahm upgrade
- *(tasks)* Track ahm context role redesign
- *(tasks)* Scope context redesign
- *(tasks)* Groom open backlog
- *(tasks)* Add 7 improvement findings from audit (130–136)
- *(tasks)* Groom backlog
- *(tasks)* Add 9 improvement findings from audit (147-155)
- *(tasks)* Groom backlog
- *(tasks)* Add tracker 156 to replace managed skills with ahm prime and procedure scopes
- *(tasks)* Groom and accept three new tasks
- *(tasks)* Record sequencing preferences for the 138/156/160 work

### Other

- *(md)* Auto-fix markdown formatting baseline

## v0.1.0 - 2026-06-17

### Added

- Scaffold initial ahm cli
- Add workflow validation to status and doctor
- Generate research and exec plan indexes
- Add task migration command
- *(tasks)* Harden task front matter parsing with strict grammar and validation
- *(tasks)* Add task status filtering and next command
- *(workflow)* Add documentation workflow guide
- *(workflow)* Validate agent artifact consistency
- *(workflow)* Add atomic writes and concurrency protection
- *(cli)* Expose AGENTS.md suggestions
- *(tasks)* Add tasks 047 and 048 for workflow validation
- *(tasks)* Stamp created and updated task metadata on mutation
- *(cli)* Add explicit human output formatters
- *(workflow)* Auto-adopt untracked managed files on init and upgrade
- *(tasks)* Support task create body input from file or stdin
- *(workflow)* Validate exec plan lifecycle
- *(tasks)* Check acceptance notes on completion
- *(tasks)* Add task work agent handoff
- *(tasks)* Default new tasks to Open and add task accept
- *(templates)* Add grooming-backlog skill template
- *(cli)* Add scoped validation modes
- *(cli)* Add opt-in project documentation health checks
- *(workflow)* Validate design-doc indexes when present
- *(tasks)* Reject unsatisfiable dependencies in task dep add
- *(tasks)* Require cancellation reasons
- *(tasks)* Capture and reuse task work agent sessions
- *(agent)* Add optional task work review orchestration with --review
- *(tasks)* Add opt-in completion handoff for task work
- *(cli)* Add task work commit handoff
- *(tasks)* Support comma-separated statuses in task list --status
- *(cli)* Switch Cake task work to stream-json orchestration
- *(cli)* Orchestrate codex task work sessions
- *(cli)* Upgrade cursor task work orchestration
- *(cli)* Use deslop review workflow for all agents
- *(tasks)* Add label-focused task listings
- *(workflow)* Add MADR ADR model
- *(tasks)* Auto-unblock dependents on completion
- *(adr)* Add adr create command
- *(adr)* Add list and show commands
- *(adr)* Add lifecycle and supersede commands
- *(adr)* Generate ADR index and validation
- *(templates)* Rewrite ADR template for MADR and update agent suggestions
- *(adr)* Add ahm adr migrate for legacy ADR metadata
- *(agent)* Add Claude Code support to ahm task work (task 082)

### Fixed

- Enforce task enum values
- Preserve optional task metadata
- Only bump version on upgrade when no conflicts remain
- Expand install dry-run preview
- Add local install recipe
- Protect AGENTS.md during workflow installs
- Make deslop template project-generic
- *(tasks)* Fix task ID resolution to avoid substring matches
- *(tasks)* Preserve unknown task front matter fields during mutations
- *(tasks)* Make task dependency cycle output deterministic
- *(cli)* Make root detection fail outside managed repositories
- *(workflow)* Always advance install metadata version despite conflicts
- *(cli)* Replace Cobra usage error string parsing with typed usageError
- *(cli,workflow)* Make index dry run report only stale generated indexes
- *(workflow)* Normalize CRLF line endings when reading workflow markdown
- *(cli)* Make status and doctor fail on validation errors
- *(tasks)* Keep task commands usable with malformed task files
- *(cli)* Remove unused --quiet and --verbose flags
- *(tasks)* Enforce dependency completion before task completion
- *(tasks)* Skip writes when status or dependency set is unchanged
- *(tasks)* Make task front matter migration index updates robust
- *(tasks)* Check for cycle before printing duplicate node in dep tree
- *(tasks)* Use consumer-neutral default task labels
- *(workflow)* Escape backticks and angle brackets in generated index tables
- *(cli)* Show none for missing installed version in status and doctor
- *(cli)* Remove doctor Go toolchain check
- *(templates)* Replace nonexistent ahm task untriaged reference
- *(workflow)* Keep install dry run side-effect free in memory
- *(templates)* Narrow agents suggestions
- *(release)* Separate binary version from template version
- *(workflow)* Fail loudly on corrupt workflow metadata instead of resetting it
- *(tasks)* Surface task parse failures during index generation
- *(cli)* Treat .git files as repository markers in root detection
- *(tasks)* Repair bucket mismatch when task status already matches
- *(tasks)* Reject YAML block list syntax in task front matter
- *(cli)* Parse type-tagged events from cake stream-json output
- *(agent)* Avoid codex review prompt conflict
- *(tasks)* Strip duplicate H1 from body-file on task create
- *(agent)* Run codex task work without sandbox prompts
- *(workflow)* Eliminate concurrent atomic write race on temp files
- *(adr)* Normalize README.md/index.md exclusion to case-insensitive
- *(adr)* Normalize supersede status check to handle non-canonical casing
- Stop pinning duplicate ADR ID blame to a single path
- *(tasks)* Serialize task id allocation
- *(task)* Remove redundant file-open instruction in task work prompt

### Changed

- Migrate CLI parsing to Cobra
- *(cli)* Standardize app methods on pointer receivers
- *(tasks)* Consolidate duplicate task DFS and collection logic
- *(cli)* Split ahm command implementation
- *(tests)* Split ahm cli tests by module
- *(tasks)* Remove splitTaskID magic numeric sentinel
- *(tasks)* Sort dependency set keys directly in taskDepUpdate
- *(tasks)* Cache task list reads to avoid redundant filesystem scans
- *(tasks)* Avoid copying seen map on every dependency tree recursion
- *(tasks)* Read each task file once during validation
- *(templates)* Make templates.Version immutable as a const
- *(tasks)* Remove dead bucketTitle branch
- *(templates)* Avoid repeated allocation of static template slices
- *(tasks)* Remove duplicate argument checks from task handlers
- *(tasks)* Normalize exec plan field in one layer only
- *(workflow)* Skip rewriting unchanged generated indexes
- *(tasks)* Reuse parsed tasks during generated index validation
- *(tasks)* Share front matter parsing helpers

### Build

- Add strict verification and release config
- Align Go toolchain with local version
- *(release)* Add binary release workflow

### Documentation

- Document cli commands and flags
- Add agent instructions
- *(workflow)* Sharpen agent handoff guidance
- *(tasks)* Record dry-run index expectation source
- *(tasks)* Expand task 008 with concrete pointers and acceptance
- Update development toolchain guidance
- Add Documentation Workflow section referencing .agents/DOCS.md
- *(templates)* Clarify task workflow guidance
- Update task workflow instructions
- *(workflow)* Add task for ahm-owned file guidance
- *(workflow)* Document ahm task create in TASKS.md workflow
- *(workflow)* Add ahm-owned file editing guidance
- *(tasks)* Document dash sentinel semantics in task fields
- *(tasks)* Record decisions in ready/pending task metadata
- *(templates)* Include grooming-backlog in important managed docs
- *(tasks)* Document canonical front matter order in spec and add round-trip tests
- *(tasks)* Align doc-validation tasks 052 and 053
- *(tasks)* Capture code review findings and reopen task 026
- Update AGENTS.md code map for write.go and task_acceptance.go
- *(adr)* Document supersession workflow
- *(tasks)* Plan MADR-only ADR management feature
- *(tasks)* Plan claude, codex, and cursor task work agent support
- *(tasks)* Plan comma-separated statuses for task list --status
- *(tasks)* Plan agent integration test harness
- Document agent integration smoke checklist in workflow docs
- *(tasks)* Capture workflow improvement tasks
- *(tasks)* Document best practices for ahm task accept
- *(tasks)* Clarify preflight upgrade removal
- *(adr)* Plan MADR ADR management
- *(tasks)* Add ahm-first workflow guidance task
- *(workflow)* Make task guidance ahm-first
- Refactor agent instructions for progressive disclosure
- Add improved Workflow Overlays section
- Document CRLF normalization in ahm adr migrate help text (task 098)
- *(adr)* Clarify supersession guidance
- Restructure documentation for progressive disclosure

### Tests

- Expand ahm cli coverage
- *(cli)* Add golden agent transcript fixtures and capture recipe
- *(cli)* Add env-gated live agent smoke test

### Maintenance

- Add ahm task management workflow
- *(tasks)* Add tasks from project review
- Add script for working on tasks
- *(tasks)* Add body-file task create feature request
- *(tasks)* Capture dry-run index bug
- Ran ahm upgrade
- Upgrade ahm in project
- *(plans)* Archive completed exec plan
- Update task-workflow to tighten up instructionsin step 3
- *(tasks)* Add task 049 to document ahm task create in TASKS.md
- *(tasks)* Add agent handoff task
- *(tasks)* Capture documentation validation follow-ups
- *(tasks)* Add task work follow-up items
- *(tasks)* Add task accept default status ticket
- *(workflow)* Refresh task workflow hash
- Run ahm upgrade
- *(tasks)* Cancel task 026
- *(ci)* Narrow gosec exclusions for file permission and path checks
- Run ahm upgrade
- Run ahm upgrade
- Add CLAUDE.md to project
- Remove superseded root working artifacts (plan.md, project-plan)
- *(tasks)* Groom backlog - block 5 tasks, move 13 to pending
- Remove dead validation-scope and output-mode helpers (069)
- Address minor CLI and rendering polish items
- *(tasks)* Groom blocked backlog
- *(tasks)* Groom active backlog
- *(tasks)* Groom agent streaming work
- *(tasks)* Track deslop review consistency
- *(tasks)* Groom backlog — move four open tasks to pending
- Update hashes in ahm.json
- *(tasks)* Add auto-unblock follow-up task
- *(tasks)* Unblock ADR follow-up tasks
- *(workflow)* Rename review skill to preflight
- Run ahm upgrade
- Add four tasks from code review findings
- Add docs-comparisons research investigation
- Groom backlog — accept four open tasks and record decisions
- *(tasks)* Add 6 maintenance tasks from codebase audit
- *(tasks)* Groom backlog — move 6 Open tasks to Pending
