package ahm

import (
	"strings"
	"testing"
)

func TestBuildTaskWorkPromptReviewAwareCompletion(t *testing.T) {
	// Verifies acceptance criteria about the initial implementation prompt:
	// when review is enabled the prompt defers completion to the finalization
	// phase; when review is disabled the prompt instructs direct completion.
	task := Task{ID: "001", Title: "Bug fix"}
	app := &app{}

	reviewPrompt := app.buildTaskWorkPrompt(task, true /*noProjectPrompt*/, true /*review*/)
	noReviewPrompt := app.buildTaskWorkPrompt(task, true, false)

	// Review path must defer completion to the finalization handoff.
	assertContainsAll(t, reviewPrompt,
		"do NOT mark the task complete yet",
		"In Progress for independent review",
		"finalization prompt to complete the task")
	assertNotContains(t, reviewPrompt, "mark the task complete with ahm")

	// No-review path must instruct direct agent-driven completion.
	assertContainsAll(t, noReviewPrompt, "mark the task complete with ahm when acceptance is satisfied")
	assertNotContains(t, noReviewPrompt, "do NOT mark the task complete yet")

	// Both paths keep the no-commit/no-push boundary.
	assertContainsAll(t, reviewPrompt, "Do not commit or push")
	assertContainsAll(t, noReviewPrompt, "Do not commit or push")
}

func TestBuildTaskWorkReviewPromptIncludesProcedureAndTaskContext(t *testing.T) {
	task := Task{ID: "156e", Title: "Embed review", Body: `## Problem

Review drifted.

## Acceptance Notes

- [ ] Prompt contains task context.
- [ ] Generated indexes remain ahm-owned.

## Comments

Not part of acceptance.`}
	prompt := buildTaskWorkReviewPrompt(task)
	assertContainsAll(t, prompt, taskWorkReviewPromptMarker, "Task: 156e — Embed review", "- [ ] Prompt contains task context.", "XS/S gets one combined pass", "M gets two", "L/XL gets three", "rules/documentation conformance", "correctness and source", "overengineering", "Managed-work completion checklist", "exec_plan", "Never commit or push")
	if strings.Contains(prompt, "Not part of acceptance") {
		t.Fatalf("prompt included content after acceptance section: %s", prompt)
	}
	if strings.Contains(prompt, "preflight skill") {
		t.Fatalf("prompt references installed skill: %s", prompt)
	}
}
