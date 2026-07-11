package ahm

import (
	"strings"
	"testing"
)

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
