# Changelog

All notable user-facing changes to `ahm` are recorded here.

## v0.3.0 - 2026-06-24

### Changed

- *(cli)* Invert `ahm task work` defaults: review and commit now run by default.
  Remove `--review` and `--commit` flags; add `--no-review` and `--no-commit`
  opt-out flags. Remove unused agent capability warnings (`supportsSessions`,
  `supportsReview`) and the unreachable non-session fallback path.

## v0.2.0 - 2026-06-24

### Changed

- *(cli)* Remove `--complete` flag from `ahm task work`. Completion is already
  part of the base work prompt; the separate `--complete` handoff is redundant.

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

