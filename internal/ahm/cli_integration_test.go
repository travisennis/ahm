package ahm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var cliIntegrationBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ahm-cli-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create integration temp dir: %v\n", err)
		os.Exit(1)
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "locate cli integration test file")
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(os.Stderr, "remove integration temp dir: %v\n", err)
		}
		os.Exit(1)
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	name := "ahm"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cliIntegrationBinary = filepath.Join(dir, name)

	cmd := exec.Command("go", "build", "-trimpath", "-o", cliIntegrationBinary, "./cmd/ahm")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build integration ahm binary: %v\n%s", err, stderr.String())
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(os.Stderr, "remove integration temp dir: %v\n", err)
		}
		os.Exit(1)
	}

	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(os.Stderr, "remove integration temp dir: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

type cliIntegrationResult struct {
	stdout string
	stderr string
	code   int
}

func runBuiltCLI(t *testing.T, dir string, args ...string) cliIntegrationResult {
	t.Helper()
	cmd := exec.Command(cliIntegrationBinary, args...) // #nosec G204 // tests run the freshly built ahm binary with explicit args
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run %s %s: %v\nstderr:\n%s", cliIntegrationBinary, strings.Join(args, " "), err, stderr.String())
		}
	}
	return cliIntegrationResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		code:   code,
	}
}

func assertIntegrationCode(t *testing.T, got cliIntegrationResult, want int) {
	t.Helper()
	if got.code != want {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", got.code, want, got.stdout, got.stderr)
	}
}

func TestCLIIntegrationExitCodes(t *testing.T) {
	root := t.TempDir()
	result := runBuiltCLI(t, root, "init")
	assertIntegrationCode(t, result, 0)

	result = runBuiltCLI(t, root)
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "installed: true", "validation:")

	result = runBuiltCLI(t, root, "--help")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "Usage:", "ahm [command]")

	result = runBuiltCLI(t, root, "boguscmd")
	assertIntegrationCode(t, result, 2)
	assertContainsAll(t, result.stderr, `unknown command "boguscmd"`)

	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "999.md"), "---\nid: 999\n---\n# Broken\n")
	result = runBuiltCLI(t, root)
	assertIntegrationCode(t, result, 1)
	assertContainsAll(t, result.stdout, `"ok": false`, `"code": "task_missing_field"`)
}

func TestCLIIntegrationTaskLifecycle(t *testing.T) {
	root := t.TempDir()
	result := runBuiltCLI(t, root, "init")
	assertIntegrationCode(t, result, 0)

	result = runBuiltCLI(t, root, "task", "create", "Integration Task", "--priority", "P1", "--effort", "M", "--description", "Created through subprocess")
	assertIntegrationCode(t, result, 0)
	if strings.TrimSpace(result.stdout) != "001" {
		t.Fatalf("task create stdout = %q, want 001", result.stdout)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"),
		"title: Integration Task",
		"status: Open",
		"priority: P1",
		"effort: M",
		"Created through subprocess",
	)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"), "Integration Task")

	result = runBuiltCLI(t, root, "task", "list")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "001 [Open] P1 M Integration Task")

	result = runBuiltCLI(t, root, "task", "show", "001")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "# Integration Task", "Created through subprocess")

	result = runBuiltCLI(t, root, "task", "accept", "001")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "001 -> Pending")

	result = runBuiltCLI(t, root, "task", "start", "001")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "001 -> In Progress")

	result = runBuiltCLI(t, root, "task", "complete", "001")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "001 -> Completed")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "status: Completed")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "index.md"), "Completed: 1")
}

func TestCLIIntegrationOutputModes(t *testing.T) {
	root := t.TempDir()
	result := runBuiltCLI(t, root, "init")
	assertIntegrationCode(t, result, 0)

	result = runBuiltCLI(t, root, "--text", "status")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "root:", "installed: true", "validation:")

	result = runBuiltCLI(t, root, "--json", "status")
	assertIntegrationCode(t, result, 0)
	var pretty map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &pretty); err != nil {
		t.Fatalf("status --json is not valid JSON: %v\n%s", err, result.stdout)
	}
	assertJSONField(t, pretty, "root")
	assertJSONField(t, pretty, "installed")
	assertJSONField(t, pretty, "validation")
	assertJSONField(t, pretty, "tasks")

	result = runBuiltCLI(t, root, "--plain", "status")
	assertIntegrationCode(t, result, 0)
	var compact map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &compact); err != nil {
		t.Fatalf("status --plain is not valid JSON: %v\n%s", err, result.stdout)
	}
	if strings.Contains(result.stdout, "\n  ") {
		t.Fatalf("status --plain should be compact JSON, got:\n%s", result.stdout)
	}
	assertJSONField(t, compact, "validation")
}

func assertJSONField(t *testing.T, object map[string]any, field string) {
	t.Helper()
	if _, ok := object[field]; !ok {
		t.Fatalf("JSON output missing field %q: %#v", field, object)
	}
}

func TestCLIIntegrationDryRunDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	result := runBuiltCLI(t, root, "init")
	assertIntegrationCode(t, result, 0)

	result = runBuiltCLI(t, root, "--dry-run", "task", "create", "Preview Task")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "create:", "id: 001")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run task create wrote active task: %v", err)
	}

	result = runBuiltCLI(t, root, "task", "create", "Real Task", "--status", "Pending")
	assertIntegrationCode(t, result, 0)

	result = runBuiltCLI(t, root, "--dry-run", "task", "complete", "001")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "move:", "status: Completed")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "001.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run task complete wrote completed task: %v", err)
	}

	result = runBuiltCLI(t, root, "--dry-run", "task", "cancel", "001", "--reason", "Superseded")
	assertIntegrationCode(t, result, 0)
	assertContainsAll(t, result.stdout, "move:", "status: Cancelled", "reason: Superseded")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "cancelled", "001.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run task cancel wrote cancelled task: %v", err)
	}
}

func TestCLIIntegrationUsageErrors(t *testing.T) {
	root := t.TempDir()
	result := runBuiltCLI(t, root, "init")
	assertIntegrationCode(t, result, 0)

	result = runBuiltCLI(t, root, "task", "create", "Bad Priority", "--priority", "P5")
	assertIntegrationCode(t, result, 2)
	assertContainsAll(t, result.stderr, `unsupported task priority "P5"`, "supported: P0, P1, P2, P3, P4")
}
