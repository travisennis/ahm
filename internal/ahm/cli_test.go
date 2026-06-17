package ahm

import (
	"strings"
	"testing"
)

func TestNestedHelp(t *testing.T) {
	stdout, stderr, code := runCLI(t, "task", "--help")
	if code != 0 {
		t.Errorf("task help exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "Manage tasks", "create", "dep", "labels")

	stdout, stderr, code = runCLI(t, "task", "create", "--help")
	if code != 0 {
		t.Errorf("task create help exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "create <title> [flags]", "--priority", "--description")
}

func TestSubcommandsRequireSubcommands(t *testing.T) {
	_, stderr, code := runCLI(t, "task")
	if code != 2 {
		t.Errorf("task exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "task requires a subcommand")

	_, stderr, code = runCLI(t, "task", "dep")
	if code != 2 {
		t.Errorf("task dep exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "task dep requires a subcommand")
}

func TestUsageErrorsExitCode2(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{
			name:    "unknown top-level command",
			args:    []string{"boguscmd"},
			message: `unknown command "boguscmd" for "ahm"`,
		},
		{
			name:    "unknown subcommand",
			args:    []string{"task", "bogus"},
			message: `unknown subcommand "bogus" for "ahm task"`,
		},
		{
			name:    "extra args to no-args command",
			args:    []string{"init", "extra"},
			message: `unknown command "extra" for "ahm init"`,
		},
		{
			name:    "extra args to version",
			args:    []string{"version", "x"},
			message: `unknown command "x" for "ahm version"`,
		},
		{
			name:    "unknown flag",
			args:    []string{"--bogus"},
			message: "unknown flag: --bogus",
		},
		{
			name:    "unknown shorthand flag",
			args:    []string{"-X"},
			message: "unknown shorthand flag: 'X' in -X",
		},
		{
			name:    "task subcommand requires subcommand",
			args:    []string{"task"},
			message: "task requires a subcommand",
		},
		{
			name:    "task dep requires subcommand",
			args:    []string{"task", "dep"},
			message: "task dep requires a subcommand",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, tt.args...)
			if code != 2 {
				t.Errorf("exit code = %d, want 2; stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.message) {
				t.Errorf("stderr missing %q:\n%s", tt.message, stderr)
			}
		})
	}
}
