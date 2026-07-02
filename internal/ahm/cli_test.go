package ahm

import (
	"strings"
	"testing"
)

func TestEmitWarningsDrains(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var buf strings.Builder
		a := app{err: &buf}
		a.emitWarnings()
		if buf.Len() != 0 {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})

	t.Run("prints and clears", func(t *testing.T) {
		var buf strings.Builder
		a := app{err: &buf}
		a.addWarning("something went wrong")
		a.addWarning("another issue")
		a.emitWarnings()
		got := buf.String()
		if !strings.Contains(got, "warning: something went wrong") {
			t.Errorf("missing first warning:\n%s", got)
		}
		if !strings.Contains(got, "warning: another issue") {
			t.Errorf("missing second warning:\n%s", got)
		}
		if len(a.warnings) != 0 {
			t.Errorf("warnings not drained: %v", a.warnings)
		}
	})

	t.Run("dedupes identical messages", func(t *testing.T) {
		var buf strings.Builder
		a := app{err: &buf}
		a.addWarning("duplicate message")
		a.addWarning("duplicate message")
		a.emitWarnings()
		got := buf.String()
		if strings.Count(got, "warning:") != 1 {
			t.Errorf("expected 1 warning line, got %d:\n%s", strings.Count(got, "warning:"), got)
		}
		if len(a.warnings) != 0 {
			t.Errorf("warnings not drained: %v", a.warnings)
		}
	})

	t.Run("nil err is no-op", func(t *testing.T) {
		a := app{}
		a.addWarning("should be ignored")
		a.emitWarnings()
		if len(a.warnings) != 1 {
			t.Errorf("warnings were drained despite nil err: %v", a.warnings)
		}
	})
}

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
