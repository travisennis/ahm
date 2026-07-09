# Documentation Workflow

This document explains how agents should update documentation in this
repository. Use it when asked to check documentation, update docs after a
change, or decide whether completed work needs documentation follow-up.

Documentation should help future contributors understand what exists, why it
exists, how to use it, and how to change it safely. Keep documentation
accurate, specific, and close to the behavior it describes.

Treat the repository's existing docs as the source for naming, structure,
tone, and level of detail: prefer existing documentation locations over
creating new ones, and prefer correcting an existing doc over adding a new
one.

## When Docs Need Updates

Update durable project documentation when a change affects:

- User-visible behavior
- Public APIs, commands, UI flows, configuration, or file formats
- Setup, installation, deployment, or operating instructions
- Security, permissions, data handling, or migration behavior
- Architecture, ownership boundaries, or durable design decisions
- Contributor workflows, testing instructions, or release process
- Known limitations, troubleshooting, or compatibility

Do not add documentation just because code changed. Internal refactors often
need no docs unless they change how people understand or work with the
project.

## Project Docs vs Agent Artifacts

Project docs are durable repository documentation intended for humans working
on or using the project. Agent artifacts are working records under
`{{.RecordsDir}}`, such as tasks, research notes, ExecPlans, and generated
indexes. Keep these roles separate: durable behavior, architecture, and
contributor guidance belong in project docs; actionable work, evidence, and
plans belong in their `{{.RecordsDir}}` records. Preserve uncertainty by
recording open questions in tasks, research, or plans instead of presenting
guesses as facts.

## Generated Indexes

Generated indexes are owned by `ahm`. Do not edit them directly. When task,
research, or ExecPlan source files change, regenerate indexes with:

```bash
ahm index
```

## Handoff

At handoff, summarize which documentation was checked, which files were
updated, which generated indexes were regenerated, and any remaining
documentation gaps or decisions needed.
