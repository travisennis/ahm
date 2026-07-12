package ahm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTaskGroomAgentSmoke verifies the structured grooming schema against the
// real supported agent CLIs. It is opt-in because every installed agent makes
// a live model call. Provider account limits are reported as unavailable.
func TestTaskGroomAgentSmoke(t *testing.T) {
	if os.Getenv("AHM_AGENT_SMOKE") != "1" {
		t.Skip("live groom smoke test; set AHM_AGENT_SMOKE=1 or run `just smoke-agents`")
	}
	for _, name := range []string{"cake", "claude", "codex", "cursor"} {
		t.Run(name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(name)
			if err != nil {
				t.Fatal(err)
			}
			executable, err := exec.LookPath(agent.executable)
			if err != nil {
				t.Skipf("%s not on PATH, skipping live groom smoke", agent.executable)
			}
			version, err := exec.Command(executable, "--version").Output()
			if err != nil {
				version = []byte("unknown")
			}
			t.Logf("%s version: %s", agent.executable, strings.TrimSpace(string(version)))

			root := setupGroomSmokeRepo(t)
			stdout, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", name)
			t.Logf("%s groom stdout evidence:\n%s", agent.executable, strings.TrimSpace(stdout))
			if code != 0 {
				if reason := liveAgentUnavailable(stdout + "\n" + stderr); reason != "" {
					t.Skipf("%s unavailable: %s", agent.executable, reason)
				}
				t.Fatalf("task groom exit code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			assertContainsAll(t, text, "status: Pending", "audit_source: smoke", "Preserve this historical note.")
			if strings.Contains(text, "- [ ] TODO") {
				t.Fatalf("agent did not replace placeholder acceptance notes:\n%s", text)
			}
		})
	}
}

func setupGroomSmokeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitInit := exec.Command("git", "init", "-q")
	gitInit.Dir = root
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if stdout, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("ahm init exit code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), `---
id: 001
title: Verify unknown task metadata preservation
status: Open
priority: P2
effort: XS
labels: type:test, area:tasks
exec_plan: -
depends_on: -
audit_source: smoke
---
# Verify unknown task metadata preservation

## Problem

The task is fully specified, but its seeded acceptance placeholder must be
replaced before it is ready. Ahm's task renderer already preserves unknown
front-matter fields in internal/ahm/tasks.go.

## Relevant Files

- internal/ahm/tasks.go — preserves unknown front-matter fields on render.

## Fix Direction

Replace the TODO acceptance item with an actionable unchecked check that says
unknown front-matter fields remain unchanged after task rendering. No product,
design, dependency, or implementation decision is missing. This revision
resolves the final gap, so the correct grooming outcome is accept, not revise.

## Historical Notes

Preserve this historical note.

## Acceptance Notes

- [ ] TODO
`)
	return root
}
