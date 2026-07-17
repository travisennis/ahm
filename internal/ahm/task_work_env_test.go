package ahm

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTaskWorkAgentEnvFilterClaudeStripsAPIKey(t *testing.T) {
	agent, err := parseTaskWorkAgent("claude")
	if err != nil {
		t.Fatal(err)
	}

	env := []string{
		"ANTHROPIC_API_KEY=secret",
		"PATH=/usr/bin",
		"HOME=/home/user",
	}
	filtered := agent.envFilter(env)
	if filtered == nil {
		t.Fatal("expected filtered env, got nil")
	}
	if containsEnvVar(filtered, "ANTHROPIC_API_KEY") {
		t.Errorf("ANTHROPIC_API_KEY should be stripped for claude")
	}
	if !containsEnvVar(filtered, "PATH") {
		t.Errorf("PATH should be preserved for claude")
	}
	if !containsEnvVar(filtered, "HOME") {
		t.Errorf("HOME should be preserved for claude")
	}
}

func TestTaskWorkAgentEnvFilterOtherAgentsUnchanged(t *testing.T) {
	for _, name := range []string{"cake", "codex", "cursor"} {
		t.Run(name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(name)
			if err != nil {
				t.Fatal(err)
			}
			if got := agent.envFilter(os.Environ()); got != nil {
				t.Errorf("%s should not filter env, got %v", name, got)
			}
		})
	}
}

func TestRunTaskWorkCommandUsesFilteredEnv(t *testing.T) {
	exe, err := exec.LookPath("env")
	if err != nil {
		t.Skip("env executable not found")
	}

	env := []string{
		"KEEP=preserved",
		"ANTHROPIC_API_KEY=secret",
	}
	filtered := filterBlockedEnv(env, []string{"ANTHROPIC_API_KEY"})
	ctx := taskWorkRunContext(taskWorkDefaultTimeout, filtered)

	var out strings.Builder
	if err := runTaskWorkCommand(ctx, t.TempDir(), exe, nil, nil, &out, nil); err != nil {
		t.Fatalf("runTaskWorkCommand: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "KEEP=preserved") {
		t.Errorf("filtered env should preserve KEEP, got output:\n%s", output)
	}
	if strings.Contains(output, "ANTHROPIC_API_KEY") {
		t.Errorf("filtered env should strip ANTHROPIC_API_KEY, got output:\n%s", output)
	}
}

func TestRunTaskWorkCommandInheritsParentEnv(t *testing.T) {
	exe, err := exec.LookPath("env")
	if err != nil {
		t.Skip("env executable not found")
	}

	t.Setenv("AHM_PARENT_ENV_TEST", "present")
	ctx := taskWorkRunContext(taskWorkDefaultTimeout, nil)

	var out strings.Builder
	if err := runTaskWorkCommand(ctx, t.TempDir(), exe, nil, nil, &out, nil); err != nil {
		t.Fatalf("runTaskWorkCommand: %v", err)
	}
	if !strings.Contains(out.String(), "AHM_PARENT_ENV_TEST=present") {
		t.Errorf("child should inherit parent env, got output:\n%s", out.String())
	}
}

func containsEnvVar(env []string, name string) bool {
	prefix := name + "="
	for _, entry := range env {
		if entry == name || strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
