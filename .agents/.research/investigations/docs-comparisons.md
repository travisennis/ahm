## Alignment vs Contradiction: `.agents/DOCS.md` ↔ `documentation-audit` skill

I read both documents in full: the project's DOCS.md, the skill's SKILL.md, rubric, principles, and report template.

---

### What They Align On

| Topic | DOCS.md | Skill | Verdict |
|---|---|---|---|
| **In-repo as source of truth** | "Prefer existing documentation locations and style" — docs live in the repo | Principle #1: "The repository is the system of record" | Aligned |
| **Single source of truth / no duplication** | Checks for "documentation that duplicates another source of truth instead of pointing to it" | Dimension #7: "Each topic has exactly one place, with links instead of duplication" | Aligned |
| **Stale / contradictory docs are bad** | Checks for stale docs and contradictions between docs and implementation | Freshness & single-source dimensions score this down | Aligned |
| **Broken links are bad** | Checks for "broken relative links" | Freshness dimension checks link resolution | Aligned |
| **Don't document every code change** | "Do not add documentation just because code changed. Internal refactors often need no docs" | No direct equivalent, but the rubric calibrates expectations to repo size — not penalizing small changes | Compatible |
| **Structured reporting** | "Include the affected file, observed problem, expected correction, fixed or open" | Report template with dimension, score, finding, recommendation | Similar structure, different detail level |

---

### Where They Contradict or Tension

#### 1. **Audience: humans vs agents**

| | DOCS.md | Skill |
|---|---|---|
| Who the docs are *for* | "intended for humans working on or using the project" | "assess whether docs serve **coding agents** well" |
| What "good" means | Accurate, specific, follows existing conventions | Directives over explanations, layered disclosure, named over linked |

**This is the root tension.** What makes prose good for a human (narrative, tutorial-style, contextual explanation) is often *bad* for an agent (crowds context, buries directives). The skill explicitly penalizes what a human maintainer might consider "thorough documentation."

#### 2. **Agent entry point: existence and shape**

| | DOCS.md | Skill |
|---|---|---|
| Mentions `AGENTS.md`/`CLAUDE.md`? | No — listed only under "project-specific documentation" as an afterthought | **Dimension #1** — single most important artifact. Absence = score 0 |
| Opinion on size/structure? | None | Strong opinion: ~100–150 lines, directive-led, links out. Penalizes monolithic AGENTS.md |

**Contradiction.** DOCS.md treats agent instruction files as optional project-specific docs. The skill treats them as the critical front door and heavily judges their quality against specific structural criteria. A repo could follow DOCS.md perfectly and still score 0–1 on `agent_entry_point` and `progressive_disclosure` because DOCS.md never tells an agent to create or care about those artifacts.

#### 3. **Progressive disclosure / layering**

DOCS.md has zero guidance on layered disclosure — no concept of an entry point linking out to ARCHITECTURE.md which links to topic docs, or nested AGENTS.md in subdirectories. The skill devotes an entire dimension to this and considers monolithic entry points an anti-pattern.

This is a **full contradiction**: the skill says "a single oversized instruction file fails" and penalizes it. DOCS.md would never flag a 400-line AGENTS.md as a problem because it has no frame of reference for what "oversized" means for an agent.

#### 4. **Directives vs explanations**

The skill's principle #3 states: "Directives over explanations — entry-point content should be rules (must/never/always/avoid/prefer), not tutorials." This is a specific stance about prose style for agent-facing docs. DOCS.md doesn't address this at all — it talks about "behavior, workflow, or decision changed" without caring whether the prose is imperative or explanatory.

#### 5. **Name vs link (architecture docs)**

| | DOCS.md | Skill |
|---|---|---|
| Link quality | Checks for broken relative links only | Matklad-inspired: "name, don't link — names survive refactors and are searchable" |

The skill actively discourages link-heavy architecture docs and says to name files/modules/types instead. DOCS.md has no concept of this trade-off and only checks whether links *work*, not whether linking is the right approach.

#### 6. **ARCHITECTURE.md content expectations**

DOCS.md lists `ARCHITECTURE*` as a discovery location and says to update it when architecture changes. That's it. The skill has a full dimension (#4) with specific content requirements: codemap, invariants (including absences), boundaries, and precise altitude guidance ("a country map, not per-function detail"). It also names anti-patterns (too detailed, too volatile, link-heavy, missing absences).

**Gap, not contradiction** — DOCS.md is silent where the skill is prescriptive. But that silence means following DOCS.md alone would produce architecture docs that might score poorly against the skill's criteria.

#### 7. **Decision records: prescription vs caution**

| | DOCS.md | Skill |
|---|---|---|
| ADR stance | Cautious: "Use ADRs only if this repository has adopted ADRs or the change warrants a durable decision record" | Dimension #8: ADRs are a scored dimension with structured format requirements |

**Tension.** DOCS.md says don't invent ADR policies the repo doesn't already use. The skill will score `decision_records` 0 if no ADRs exist, then recommend adding them. A repo following DOCS.md's caution might intentionally have no ADRs; the skill would see that as a gap.

#### 8. **Mechanical enforcement**

DOCS.md has zero mentions of CI checks, link checkers, doc linters, freshness gates, or doc-coverage verification. The skill's dimension #9 treats mechanical enforcement as the differentiator between a 2 (solid but manual) and 3 (strong and protected). This is a **full gap** — nothing in DOCS.md would ever prompt an agent to set up or maintain CI checks for documentation quality.

#### 9. **Scoring rigor**

DOCS.md uses three informal severity levels (error/warning/info) with no aggregation or grade. The skill uses a 0–3 scored rubric across nine dimensions with a computed percentage and grade band (Opaque/Sparse/Navigable/Legible). These aren't contradictory — they serve different needs — but they're incompatible as a single framework.

#### 10. **Scope of doc audit checklist**

DOCS.md's audit checklist is short and practical: missing docs, stale docs, contradictions, broken links, orphaned artifacts, stale indexes, status conflicts, duplication. The skill's nine dimensions cover all of those but also add: agent entry point quality, progressive disclosure, command documentation completeness and parseability, architecture codemap with invariants and absences, domain knowledge capture, docs organization/indexing, ADR practice, and mechanical enforcement. The skill is **substantially more comprehensive**.

---

### Root Cause

These documents were written for different jobs:

- **DOCS.md** is a *process guide for a human contributor* updating documentation after a change. It answers "when and how should I touch docs?"
- **documentation-audit** is an *evaluation framework for an agent* assessing a codebase's doc quality. It answers "how well does this repo's documentation serve an agent?"

They coexist because they're meant for different phases of work — DOCS.md for maintenance, the skill for assessment — but they disagree on what "good" looks like because they optimize for different consumers (humans vs agents). The most concrete contradictions are around agent entry points (DOCS.md doesn't care about them; the skill treats them as critical) and the most important gaps are around progressive disclosure, directives-style prose, naming-over-linking in architecture docs, and mechanical enforcement — all absent from DOCS.md.

**Implication:** If this repo wants docs to score well on an agent-legibility audit, DOCS.md needs updating. It currently doesn't instruct agents to create or maintain the artifacts the skill considers most important for agent navigation.
