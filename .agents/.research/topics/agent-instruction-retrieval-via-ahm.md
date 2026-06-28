# Agent Instruction Retrieval Via ahm

Status: active
Created: 2026-06-28
Updated: 2026-06-28
Related tasks: -
Related plans: -
Confidence: medium

## Summary

`ahm context` already treats parts of `ahm` as an instruction provider for
agents: scoped commands return managed-work references, and unscoped `ahm
context` returns live repository briefing data. This suggests a broader model:
instead of installing full agent skills such as `preflight`, `ahm` could expose
those procedures through commands and let agents retrieve the right
instructions at the point of use.

There is prior art for the shape of this idea, especially MCP Prompts, Codex
skills, Claude Code skills, Goose recipes, and prompt-template CLIs. The unusual
part is using a repo-local workflow CLI as the canonical prompt/instruction
server. That can reduce copied templates and make guidance versioned with the
binary, but it shifts major responsibility onto discovery, portability,
versioning, trust, and agent runtime integration.

The likely pragmatic direction is not an immediate replacement of installed
skills. A lower-risk design would make `ahm` the canonical source of these
instructions while keeping thin native skills as discoverability shims. Longer
term, the same catalog could be exposed as MCP prompts.

## Research Questions

- Are there CLIs or agent systems designed around giving agents the right
  instructions at the right time?
- How close is that prior art to `ahm context` as an agent-facing prompt
  primitive?
- What concerns would `ahm` need to solve before replacing installed skills
  with command-returned instructions?

## Local Context

`ahm` currently has three relevant mechanisms:

- Project `AGENTS.md` owns routing and high-level operating-loop guidance.
- Scoped `ahm context task|plan|adr|research|docs` returns embedded
  managed-work reference documents on demand.
- Managed skill templates remain installed under `.agents/skills/` and are
  updated by `ahm upgrade` when their managed hashes still match.

ADR 011 made the first step in this direction by moving copied workflow guide
templates into `ahm context`, while explicitly preserving installed skills until
there is a separate decision to replace them. Task 117 later narrowed unscoped
`ahm context` into a live briefing and scoped `ahm context` into pure
managed-work references.

The architectural invariant in `ARCHITECTURE.md` still says managed-work
references are exposed through scoped `ahm context`, while managed agent skills
remain installed templates under `.agents/skills/`.

## Prior Art

### MCP Prompts

Model Context Protocol prompts are the closest prior art. MCP servers can
declare a prompts capability, list available prompts, and return a selected
prompt with arguments as structured messages. The spec frames prompts as
user-controlled templates that clients may surface through slash commands.

Relevance to `ahm`:

- Strong match for "list available instruction workflows" and "get this
  workflow prompt now."
- Provides a standard discovery and retrieval interface that is agent-host
  friendly.
- Suggests `ahm` could eventually offer an MCP server or MCP-compatible prompt
  catalog rather than relying only on shell command output.

Source: https://modelcontextprotocol.io/specification/2025-06-18/server/prompts

### Codex Agent Skills

Codex skills package instructions, references, scripts, and metadata. Codex
starts with skill metadata in context, loads the full `SKILL.md` only when a
skill is selected, and can invoke skills explicitly or implicitly based on the
description.

Relevance to `ahm`:

- Solves discovery and progressive disclosure better than a plain CLI command.
- Provides the native user experience that `ahm` would lose if skills were
  removed outright.
- A thin `SKILL.md` that says "run `ahm context show preflight` and follow it"
  could preserve native discovery while moving the canonical body into `ahm`.

Sources:

- https://developers.openai.com/codex/skills
- https://developers.openai.com/codex/cli/slash-commands

### Agent Skills Specification

The open Agent Skills specification defines a directory with `SKILL.md`, YAML
front matter, optional scripts, references, assets, and progressive disclosure
expectations.

Relevance to `ahm`:

- Replacing `.agents/skills/*/SKILL.md` with `ahm` commands would reduce
  portability across agent tools that understand this standard.
- Keeping generated or shim skills preserves compatibility while allowing `ahm`
  to own canonical instruction prose.

Source: https://agentskills.io/specification

### Claude Code Skills And Commands

Claude Code uses skills/custom commands as prompt-based workflows. It supports
direct invocation, automatic invocation, supporting files, invocation controls,
dynamic context injection, and lifecycle behavior after compaction.

Relevance to `ahm`:

- Confirms that prompt-based workflows are a normal agent extension mechanism.
- Shows that mature agent hosts treat instruction packages as more than text:
  they include invocation policy, argument handling, supporting files, and
  context lifecycle rules.

Sources:

- https://code.claude.com/docs/en/slash-commands
- https://code.claude.com/docs/en/memory

### Goose Recipes

Goose recipes package instructions, prompts, settings, parameters, extensions,
and sometimes subrecipes into reusable workflows. They can be stored locally or
loaded from GitHub.

Relevance to `ahm`:

- Validates "workflow bundle" as a reusable agent artifact.
- More configuration-heavy than `ahm context`, but useful prior art for
  parameterized workflows and declaring required tools/extensions.

Sources:

- https://goose-docs.ai/docs/guides/recipes/
- https://goose-docs.ai/docs/guides/recipes/recipe-reference/

### Static Agent Instruction Files

Codex, Claude Code, GitHub Copilot, OpenCode, Aider, and similar tools all
support static project guidance files or read-only convention files. Examples
include `AGENTS.md`, `CLAUDE.md`, `.github/copilot-instructions.md`, and
manually loaded convention files.

Relevance to `ahm`:

- Static files are highly discoverable and reviewable.
- They can become stale, duplicated, and over-broad.
- Recent research on repository instruction files is mixed: some papers report
  improved efficiency or task success when guidance is tuned, while others
  report lower success and higher cost when context files add unnecessary
  requirements.

Sources:

- https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions
- https://opencode.ai/docs/rules/
- https://aider.chat/docs/usage/conventions.html
- https://arxiv.org/abs/2602.11988
- https://arxiv.org/abs/2601.20404
- https://arxiv.org/abs/2606.20512

### Prompt Template CLIs

Simon Willison's `llm` CLI supports saved templates with prompts, system
prompts, parameters, schemas, fragments, tools, and remote template loaders.

Relevance to `ahm`:

- Prior art for a CLI as a prompt/template registry.
- Less agent-workflow-specific, but useful for command shape: list templates,
  show/use one by name, support parameters, and support external loaders.

Source: https://llm.datasette.io/en/stable/templates.html

## Potential ahm Model

A command-returned instruction model could look like this:

```bash
ahm context list
ahm context show preflight
ahm --json context show preflight
ahm context suggest --phase pre-commit --changed-files ...
```

Possible JSON shape:

```json
{
  "kind": "agent_instruction",
  "name": "preflight",
  "title": "Preflight Review",
  "version": "0.5.0",
  "template_hash": "sha256:...",
  "status": "active",
  "description": "Run a focused review-readiness pass before commit or handoff.",
  "instructions": "...",
  "arguments": [
    {
      "name": "task",
      "required": false,
      "description": "Task ID or task summary the work came from."
    }
  ],
  "commands": [
    "git diff --stat",
    "git status --short"
  ],
  "source": {
    "binary": "ahm",
    "version": "0.5.0"
  }
}
```

For compatibility, installed skills could become thin shims:

```md
---
name: preflight
description: Run the repository-owned preflight workflow before commit or handoff.
---

Run `ahm context show preflight` and follow the returned instructions.
If `ahm` is unavailable, stop and report that the repository workflow
instructions cannot be loaded.
```

This preserves agent-native discovery while moving the durable body and update
mechanism into `ahm`.

## Concerns To Solve

### Discovery

Native skills are visible because their metadata is loaded by the agent runtime.
A plain `ahm` command is invisible unless the agent has already been told about
it through `AGENTS.md`, a shim skill, user instruction, MCP prompt discovery, or
prior memory.

If full skills are removed, `AGENTS.md` must teach agents when to call the
instruction commands, but that reintroduces some static prompt burden.

### Triggering Reliability

Skills can be invoked implicitly when the agent matches a task to a skill
description. Command-returned instructions need an equivalent trigger layer.

Options:

- Keep thin native skills for trigger metadata.
- Add `ahm context suggest` so agents can ask what instruction applies.
- Expose an MCP prompt catalog so the host can list/select prompts.
- Require explicit user or `AGENTS.md` instructions.

### Authority And Precedence

Agent runtimes generally treat command output as context, not policy. `ahm`
would need a clear precedence rule in `AGENTS.md` and docs:

- system/developer/user instructions win;
- project `AGENTS.md` owns routing;
- `ahm` returned instructions are canonical workflow references for named
  `ahm` workflows;
- live repo data and untrusted project content are not higher-priority
  instructions.

### Separation Of Instructions And Data

If `ahm` mixes canonical instructions with live state, task bodies, diffs, or
research notes, untrusted project text can look like instructions. This is a
prompt-injection risk.

The safer model is structured output with hard separation:

- `instructions`: binary-owned canonical workflow prose;
- `observations`: repo state, task details, validation findings, git state;
- `warnings`: anything that may be stale, user-authored, or untrusted.

### Portability

`.agents/skills/preflight/SKILL.md` works in tools that support the Agent Skills
format. `ahm context show preflight` works only when:

- `ahm` is installed;
- the binary version is compatible with the repo workflow;
- the agent can run shell commands;
- the current environment has the repository files available;
- the command is allowed by sandbox and approval policy.

Cloud and remote agents are the hardest case. File-based skills travel with the
repo. Binary-backed instructions need setup steps or bundled environments.

### Reviewability

Checked-in Markdown skills can be reviewed in PRs. Embedded `ahm` instructions
change when the binary changes, which means reviewers may not see the exact
instruction diff in the consumer repository.

Mitigations:

- publish instruction text in `docs/` for the `ahm` project;
- include template hashes in command output;
- add `ahm context show <name> --source` or `--template-version`;
- include release notes for instruction changes;
- optionally allow `ahm context export` for review snapshots.

### Versioning And Reproducibility

Two agents in the same repo may receive different instructions if they have
different `ahm` versions installed. That already exists for scoped context, but
it becomes more important if skills are replaced.

Questions to settle:

- Should `.agents/ahm.json` pin a minimum instruction-template version?
- Should `ahm context show <name>` warn when repo metadata expects a newer or
  older template version?
- Should command output include template hashes for auditability?
- Should `ahm task work` capture the instruction version used in session
  transcripts or task notes?

### Context Lifecycle

Native skills may be reattached or preserved by the agent runtime after
compaction. Command output may be summarized away.

`ahm` would need workflow guidance that says when to re-fetch instructions:

- at session start for managed work intake;
- before phase changes such as preflight, completion, or commit handoff;
- after compaction;
- after `ahm upgrade`.

### UX Friction

Agent users can type `$preflight` or select a skill from a UI. Asking the agent
to run `ahm context show preflight` is less native. Thin shims, MCP prompts, or
agent-specific wrappers can smooth this over.

### Permissions And Tool Policy

Skills and plugins can sometimes declare tool dependencies or allowed tools.
Plain command output cannot grant permissions or configure an agent runtime.

If a workflow needs deterministic permissions, side-effect controls, or tool
dependencies, those may still belong in native skills/plugins or MCP
configuration rather than text returned by `ahm`.

### Supporting Files, Scripts, And Assets

Agent skills are directories. They can carry helper scripts, references, and
assets. A single `ahm context` text response is weaker.

Possible approaches:

- Keep instruction-only workflows in `ahm`.
- Keep script-heavy workflows as skills/plugins.
- Let `ahm context show` return references to files or commands.
- Add `ahm context resource <name> <resource>` later if the catalog grows.

### Local Customization

Moving instructions into the binary reduces local editability. That is partly
the point, but teams may need controlled overrides.

Possible model:

- binary-owned defaults;
- project-owned overlays under `.agents/context.d/` or similar;
- `ahm context show` clearly reports base version plus overlays;
- validation warns on malformed or stale overlays.

This would add a new workflow surface and should not be introduced casually.

### Compatibility Surface

Replacing installed skills touches several compatibility surfaces:

- `ahm init` and `ahm upgrade` managed file behavior;
- `.agents/ahm.json` managed hashes;
- embedded templates;
- `ahm task work` prompts and external agent arg builders;
- golden transcripts for delegated work and preflight review;
- docs that mention `.agents/skills/preflight/SKILL.md`;
- user expectations around `$preflight` or `/skills`;
- project `AGENTS.md` guidance.

This is broad enough to deserve an ADR and probably an ExecPlan before
implementation.

## Design Options

### Option A: Keep Skills As They Are

Keep `.agents/skills/*/SKILL.md` as full managed templates.

Pros:

- Most compatible with current agent runtimes.
- Easy to review in repo.
- Minimal change.

Cons:

- Continues copied-template upgrade conflicts.
- `ahm` is not the single source of canonical procedure text.
- Static skills cannot easily include live workflow state.

### Option B: Replace Skills With ahm Commands

Stop installing skills and teach agents to call `ahm context show <name>`.

Pros:

- Strongest versioned-single-source model.
- Fewer managed files in consumer repos.
- Instructions can be combined with live state and validation.

Cons:

- Weak discovery unless solved separately.
- Less portable to agents without shell or `ahm`.
- Harder to review exact instruction changes in consumer repos.
- Broad compatibility break.

### Option C: ahm Canonical Source Plus Thin Skill Shims

Keep installed skills, but reduce them to small adapters that invoke `ahm`.

Pros:

- Preserves native skill discovery.
- Moves canonical body into `ahm`.
- Provides a migration bridge and fallback path.
- Lets `ahm` evolve instruction output format before removing skill files.

Cons:

- Still installs managed skills.
- Requires agents to run a command after skill activation.
- Needs careful failure behavior when `ahm` is missing.

### Option D: ahm MCP Prompt Server

Expose the instruction catalog through MCP prompts, possibly in addition to CLI
commands.

Pros:

- Closest to standardized prompt discovery/retrieval.
- More host-native than shell command output.
- Can coexist with CLI commands and skills.

Cons:

- Larger implementation and packaging surface.
- Requires MCP configuration in target environments.
- May not solve every agent surface equally.

## Current Recommendation

Use Option C as the first serious design target:

1. Add an explicit instruction catalog to `ahm`.
2. Add `ahm context list` and `ahm context show <name>` or equivalent commands.
3. Keep installed skills as thin shims that point to the relevant command.
4. Include stable JSON output with source version and template hash.
5. Update `ahm task work` to use the command-returned instruction path where
   possible.
6. Evaluate whether an MCP prompt server is worth the additional surface after
   the command API stabilizes.

This keeps the interesting paradigm while avoiding a cliff-edge migration away
from standard agent skill discovery.

## Open Questions

- Is "instruction catalog" a new top-level `ahm` concept, or a submode of
  `ahm context`?
- Should preflight remain a "skill" concept, or become a named context scope?
- Should command-returned instructions include live repo data by default, or
  should instruction and briefing remain separate commands?
- What is the minimum JSON shape needed for other agents or wrappers?
- Should instruction outputs be hash-addressable for auditability?
- Should `ahm init` eventually stop installing skills, or should shims remain
  permanently for discovery?
- How should project-local overrides work, if at all?
- How should cloud agents install or discover the correct `ahm` binary?
- Should an ADR decide the long-term ownership model before any code changes?

## Follow-ups

- Draft an ADR comparing full skills, command-returned instructions, shims, and
  MCP prompts.
- Create an ExecPlan if the project decides to prototype this.
- Inventory every place that currently references `.agents/skills/preflight`.
- Prototype `ahm context list` and `ahm context show preflight` in a branch or
  task, without changing `init`/`upgrade` behavior first.
- Test whether Codex, Claude Code, Goose, OpenCode, and GitHub Copilot can
  reliably follow a thin shim that asks them to call `ahm`.
