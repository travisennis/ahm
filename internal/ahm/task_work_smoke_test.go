package ahm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentSmoke runs the real agent CLIs end-to-end through `ahm task work`.
// It exists because golden fixtures (task 088) verify output parsing but not
// argument construction: whether the flag shapes the arg builders emit are
// accepted by the installed binaries and whether a captured session ID is
// actually resumable. Those can only be checked by running the real agents.
//
// The test is gated on AHM_AGENT_SMOKE=1 and skipped otherwise, so `just ci`
// never runs it. Run it with `just smoke-agents` after changing taskWorkAgent
// arg builders, output parsers, or the orchestration flow in task_agents.go,
// task_parsers.go, or task_session.go. Each installed agent costs a few real
// LLM calls (one work session plus one resume); agents not on PATH are skipped
// individually.
func TestAgentSmoke(t *testing.T) {
	if os.Getenv("AHM_AGENT_SMOKE") != "1" {
		t.Skip("live agent smoke test; set AHM_AGENT_SMOKE=1 or run `just smoke-agents`")
	}
	// Session-capable agents only: the assertions cover session capture and
	// resume.
	for _, name := range []string{"cake", "claude", "codex", "cursor"} {
		t.Run(name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(name)
			if err != nil {
				t.Error(err)
			}
			executable, err := exec.LookPath(agent.executable)
			if err != nil {
				t.Skipf("%s not on PATH, skipping live smoke", agent.executable)
			}
			version, verr := exec.Command(executable, "--version").Output()
			if verr != nil {
				version = []byte("unknown")
			}
			t.Logf("%s version: %s", agent.executable, strings.TrimSpace(string(version)))

			root := setupSmokeRepo(t)
			// Verifies session capture and agent args against a real session ID.
			stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", name)
			t.Logf("%s stderr evidence:\n%s", agent.executable, liveSmokeStderrEvidence(stderr))
			if code != 0 {
				if reason := liveAgentUnavailable(stdout + "\n" + stderr); reason != "" {
					t.Skipf("%s unavailable: %s", agent.executable, reason)
				}
				t.Errorf("task work exit code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			assertContainsAll(t, stderr, "session started:")
			assertNotContains(t, stderr,
				"no session ID returned by",
				"could not capture session ID")
		})
	}
}

func liveAgentUnavailable(output string) string {
	for _, reason := range []string{"Credit balance is too low", "hit your usage limit"} {
		if strings.Contains(output, reason) {
			return reason
		}
	}
	return ""
}

func liveSmokeStderrEvidence(stderr string) string {
	var evidence []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "session started:") ||
			strings.Contains(line, "no session ID returned by") ||
			strings.Contains(line, "could not capture session ID") {
			evidence = append(evidence, line)
		}
	}
	if len(evidence) > 0 {
		return strings.Join(evidence, "\n")
	}
	return strings.TrimSpace(stderr)
}

// setupSmokeRepo builds a throwaway git repository with the ahm workflow
// installed and one explicit do-nothing task, mirroring the environment
// `ahm task work` runs in for real.
func setupSmokeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitInit := exec.Command("git", "init", "-q")
	gitInit.Dir = root
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Errorf("git init: %v: %s", err, out)
	}
	if stdout, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Errorf("ahm init exit code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), `---
id: 001
title: Agent smoke no-op task
status: Pending
priority: P2
effort: XS
labels: type:test
exec_plan: -
---
# Agent smoke no-op task

## Summary

This is an automated smoke test of the agent harness. There is no work to do.
Do not create, modify, or delete any files. Do not run any commands. For this
prompt and every follow-up prompt in this session, including requests to
review, complete, verify, or commit the task, reply with the single word:
done.

## Acceptance Notes

- [x] No action required.
`)
	return root
}
