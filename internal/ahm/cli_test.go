package ahm

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
)

func TestParseFrontMatter(t *testing.T) {
	meta, body, err := parseFrontMatter("---\nid: 001\ntitle: Test Task\n---\n# Test Task\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta["id"] != "001" {
		t.Fatalf("id = %q", meta["id"])
	}
	if meta["title"] != "Test Task" {
		t.Fatalf("title = %q", meta["title"])
	}
	if !strings.Contains(body, "# Test Task") {
		t.Fatalf("body = %q", body)
	}
}

func TestParseFrontMatter_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr string
	}{
		{
			name:  "no front matter",
			input: "# Just a heading\n",
			want:  map[string]string{},
		},
		{
			name:  "empty front matter",
			input: "---\n---\n# Body\n",
			want:  map[string]string{},
		},
		{
			name:  "values with colons",
			input: "---\nlabels: type:bug, area:tasks\ndepends_on: 001, 002\n---\n# Task\n",
			want: map[string]string{
				"labels":     "type:bug, area:tasks",
				"depends_on": "001, 002",
			},
		},
		{
			name:  "double-quoted values",
			input: "---\ntitle: \"My Task: The Reckoning\"\n---\n# Task\n",
			want: map[string]string{
				"title": "My Task: The Reckoning",
			},
		},
		{
			name:  "comment lines are skipped",
			input: "---\nid: 001\n# this is a comment\ntitle: Task\n---\n# Body\n",
			want: map[string]string{
				"id":    "001",
				"title": "Task",
			},
		},
		{
			name:  "empty lines are skipped",
			input: "---\nid: 001\n\ntitle: Task\n\n---\n# Body\n",
			want: map[string]string{
				"id":    "001",
				"title": "Task",
			},
		},
		{
			name:    "block scalar pipe rejected",
			input:   "---\ndescription: |\n  multi\n  line\n---\n# Body\n",
			wantErr: "unsupported block scalar",
		},
		{
			name:    "block scalar gt rejected",
			input:   "---\nnote: >\n  folded\n  text\n---\n# Body\n",
			wantErr: "unsupported block scalar",
		},
		{
			name:    "invalid key with spaces",
			input:   "---\nbad key: value\n---\n# Body\n",
			wantErr: "invalid front matter key",
		},
		{
			name:  "indented comment lines",
			input: "---\nid: 001\n # indented comment\ntitle: Task\n---\n# Body\n",
			want: map[string]string{
				"id":    "001",
				"title": "Task",
			},
		},
		{
			name:  "extra whitespace around key",
			input: "---\n  id  : 001\n  title : Task\n---\n# Body\n",
			want: map[string]string{
				"id":    "001",
				"title": "Task",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := parseFrontMatter(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len(meta) = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("meta[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestStripHeading(t *testing.T) {
	got := stripHeading("\n# Test Task\n\n## Summary\n\nBody\n", "Test Task")
	if !strings.HasPrefix(got, "## Summary") {
		t.Fatalf("body = %q", got)
	}
}

func TestNextTaskID(t *testing.T) {
	got := nextTaskID([]Task{{ID: "001"}, {ID: "002a"}, {ID: "010"}})
	if got != "011" {
		t.Fatalf("nextTaskID = %q", got)
	}
}

func TestResolveTask(t *testing.T) {
	root := t.TempDir()
	initDir := filepath.Join(root, ".agents", ".tasks")
	for _, dir := range []string{"active", "completed", "cancelled"} {
		if err := os.MkdirAll(filepath.Join(initDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Task One", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Task Two", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "010.md"), "010", "Task Ten", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "011.md"), "011", "Task Eleven", "Pending", "")
	a := app{opts: options{root: root}}

	t.Run("exact match", func(t *testing.T) {
		task, err := a.resolveTask("001")
		if err != nil {
			t.Fatal(err)
		}
		if task.ID != "001" {
			t.Fatalf("id = %q", task.ID)
		}
	})

	t.Run("exact match second form", func(t *testing.T) {
		task, err := a.resolveTask("010")
		if err != nil {
			t.Fatal(err)
		}
		if task.ID != "010" {
			t.Fatalf("id = %q", task.ID)
		}
	})

	t.Run("numeric prefix matches zero-padded", func(t *testing.T) {
		task, err := a.resolveTask("1")
		if err != nil {
			t.Fatal(err)
		}
		if task.ID != "001" {
			t.Fatalf("id = %q", task.ID)
		}
	})

	t.Run("numeric prefix with leading zero", func(t *testing.T) {
		task, err := a.resolveTask("01")
		if err != nil {
			t.Fatal(err)
		}
		if task.ID != "001" {
			t.Fatalf("id = %q", task.ID)
		}
	})

	t.Run("numeric prefix matches 010", func(t *testing.T) {
		task, err := a.resolveTask("10")
		if err != nil {
			t.Fatal(err)
		}
		if task.ID != "010" {
			t.Fatalf("id = %q", task.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := a.resolveTask("999")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("pattern zero not found", func(t *testing.T) {
		// "0" would match all tasks via string prefix, but only matches
		// numerically if a task has numeric part 0. None do, so expect not found.
		_, err := a.resolveTask("0")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found, got: %v", err)
		}
	})

	t.Run("ambiguous 01 when multiple tasks share numeric part", func(t *testing.T) {
		// Add a task with a suffix that also has num=1
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001a.md"), "001a", "Task One A", "Pending", "")
		_, err := a.resolveTask("1")
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("expected ambiguous, got: %v", err)
		}
	})
}

func TestTaskCreateAllowsFlagsAfterTitle(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Smoke", "task", "--description", "Verify task creation", "--priority", "P1")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"),
		"title: Smoke task",
		"priority: P1",
		"Verify task creation",
	)
}

func TestTaskCreateRejectsUnsupportedEnums(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "status",
			args: []string{"Smoke task", "--status", "Doing"},
			want: `unsupported task status "Doing"`,
		},
		{
			name: "priority",
			args: []string{"Smoke task", "--priority", "P5"},
			want: `unsupported task priority "P5"`,
		},
		{
			name: "effort",
			args: []string{"Smoke task", "--effort", "XXL"},
			want: `unsupported task effort "XXL"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			stdout, stderr, code := runCLI(t, "--root", root, "init")
			if code != 0 {
				t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}

			_, stderr, code = runCLI(t, append([]string{"--root", root, "task", "create"}, tt.args...)...)
			if code != 2 {
				t.Fatalf("exit code = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestParseTaskRejectsUnsupportedEnums(t *testing.T) {
	path := filepath.Join(t.TempDir(), "001.md")
	content := "---\n" +
		"id: 001\n" +
		"title: Bad Task\n" +
		"status: Pending\n" +
		"priority: Urgent\n" +
		"effort: S\n" +
		"labels: type:task\n" +
		"exec_plan: -\n" +
		"depends_on: []\n" +
		"---\n" +
		"# Bad Task\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := parseTask(path, "active")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `unsupported task priority "Urgent"`) {
		t.Fatalf("error = %q", err)
	}
}

func TestTaskMigrateDryRunReportsLegacyFrontMatterFixes(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	content := "---\n" +
		"id: 001\n" +
		"title: Legacy Task\n" +
		"status: Pending\n" +
		"priority: -\n" +
		"effort: XL (split into 001a / 001b)\n" +
		"exec_plan: -\n" +
		"depends_on: 050 (Backend abstraction, completed), 051 (Tool abstraction, completed)\n" +
		"---\n" +
		"# Legacy Task\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.taskMigrate(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"migrations:",
		"  .agents/.tasks/active/001.md:",
		"    - add labels",
		"    - set priority placeholder to P3",
		"    - normalize effort to XL",
		"    - normalize depends_on",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, got)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "labels:") {
		t.Fatalf("dry-run changed task file:\n%s", data)
	}
}

func TestTaskMigrateWritesMechanicalFixes(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	content := "---\n" +
		"id: 001\n" +
		"title: Legacy Task\n" +
		"status: Pending\n" +
		"priority: -\n" +
		"effort: -\n" +
		"exec_plan: -\n" +
		"depends_on: \"-\"\n" +
		"---\n" +
		"# Legacy Task\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskMigrate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "migrated 1 task files") {
		t.Fatalf("stdout = %q", out.String())
	}
	assertFileContainsAll(t, path,
		"priority: P3",
		"effort: M",
		"labels: type:task, area:unknown",
		"depends_on: -",
	)
}

func TestNormalizeDependsOnValueHandlesLegacyAnnotations(t *testing.T) {
	got, changed := normalizeDependsOnValue("030 (existing - see notes), 059 (Output sink; completed, does not supersede this task).")
	if !changed || got != "030, 059" {
		t.Fatalf("normalizeDependsOnValue = %q, %v", got, changed)
	}
	got, changed = normalizeDependsOnValue("Follows 061")
	if !changed || got != "061" {
		t.Fatalf("normalizeDependsOnValue follows = %q, %v", got, changed)
	}
	got, changed = normalizeDependsOnValue("Completed by 061.")
	if !changed || got != "061" {
		t.Fatalf("normalizeDependsOnValue completed by = %q, %v", got, changed)
	}
	got, changed = normalizeDependsOnValue("Resolved in same PR as 110 with 089.")
	if !changed || got != "-" {
		t.Fatalf("normalizeDependsOnValue note = %q, %v", got, changed)
	}
}

func TestStatusReportsValidationFindings(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "TASKS.md"), []byte("locally changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Blocked Task", "Pending", "depends_on: 999\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Cycle A", "Pending", "depends_on: 003\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Cycle B", "Pending", "depends_on: 002\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`"ok": false`,
		`"code": "managed_file_modified"`,
		`"path": ".agents/TASKS.md"`,
		`"code": "task_dependency_missing"`,
		`task 001 depends on missing task 999`,
		`"code": "task_dependency_cycle"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorReportsMalformedTaskEnums(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Bad Task", "Doing", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`"workflow_installed": true`,
		`"ok": false`,
		`"code": "task_malformed"`,
		`unsupported task status \"Doing\"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestStatusWithoutMetadataDoesNotCascadeWorkflowArtifactFindings(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got, `"code": "metadata_missing"`)
	assertNotContains(t, got,
		`"code": "generated_index_missing"`,
		`"code": "markdown_link_missing"`,
	)
}

func TestStatusReportsWorkflowArtifactConsistency(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: []\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "new-note.md"), "# New Note\n\nThis should make the research index stale.\n")
	if err := os.Remove(filepath.Join(root, ".agents", ".tasks", "cancelled", "index.md")); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_bucket_mismatch"`,
		`completed task should be in .agents/.tasks/completed`,
		`"code": "generated_index_missing"`,
		`"path": ".agents/.tasks/cancelled/index.md"`,
		`"code": "generated_index_stale"`,
		`"path": ".agents/.research/index.md"`,
	)
}

func TestStatusReportsCompletedTaskReferencingActiveExecPlan(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "---\n"+
		"id: 001\n"+
		"title: Plan Still Active\n"+
		"status: Completed\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task\n"+
		"exec_plan: rollout\n"+
		"depends_on: []\n"+
		"---\n"+
		"# Plan Still Active\n\n"+
		"## Summary\n\nDone.\n")
	writeFile(t, filepath.Join(root, ".agents", "exec-plans", "active", "rollout.md"), "# Rollout\n\n## Outcomes & Retrospective\n\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(),
		`"code": "task_completed_exec_plan_active"`,
		`completed task 001 references active ExecPlan .agents/exec-plans/active/rollout.md`,
	)
}

func TestStatusReportsMarkdownLinksInWorkflowFiles(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "links.md"), "# Links\n\n[missing](missing.md)\n\n```md\n[ignored](also-missing.md)\n```\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`"path": ".agents/.research/topics/links.md:3"`,
		`relative Markdown link target does not exist: missing.md`,
	)
	assertNotContains(t, got, "also-missing.md")
}

func TestInstallDryRunPreviewsAllWrites(t *testing.T) {
	root := t.TempDir()
	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.install(false); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{
		"created:",
		"  AGENTS.md",
		"directories:",
		"  .agents/.tasks/active",
		"metadata:",
		"  .agents/ahm.json",
		"indexes:",
		"  .agents/.tasks/active/index.md",
		"  .agents/.tasks/cancelled/index.md",
		"  .agents/.tasks/completed/index.md",
		"  .agents/.tasks/index.md",
		"  .agents/.research/index.md",
		"  .agents/exec-plans/active/index.md",
		"  .agents/exec-plans/completed/index.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote .agents directory, err = %v", err)
	}
}

func TestInstallWritesExpectedScaffoldOutput(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"created:",
		"  AGENTS.md",
		"  .agents/DOCS.md",
		"  .agents/TASKS.md",
		"  .agents/skills/deslop/SKILL.md",
		"  docs/adr/README.md",
	)

	assertFileContainsAll(t, filepath.Join(root, ".agents", "ahm.json"),
		`"version": "`+templates.Version+`"`,
		`".agents/DOCS.md":`,
		`".agents/TASKS.md":`,
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "index.md"),
		"# Task Index",
		"- Pending: 0",
		"## Next Ready Queue",
		"None.",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".research", "index.md"),
		"# Research Index",
		"This file is generated by `ahm index`.",
		"No inbox research documents yet.",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", "exec-plans", "active", "index.md"),
		"# Active ExecPlans",
		"This file is generated by `ahm index`.",
		"No active ExecPlans yet.",
	)
}

func TestNestedHelp(t *testing.T) {
	stdout, stderr, code := runCLI(t, "task", "--help")
	if code != 0 {
		t.Fatalf("task help exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "Manage tasks", "create", "dep")

	stdout, stderr, code = runCLI(t, "task", "create", "--help")
	if code != 0 {
		t.Fatalf("task create help exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "create <title> [flags]", "--priority", "--description")
}

func TestSubcommandsRequireSubcommands(t *testing.T) {
	_, stderr, code := runCLI(t, "task")
	if code != 2 {
		t.Fatalf("task exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "task requires a subcommand")

	_, stderr, code = runCLI(t, "task", "dep")
	if code != 2 {
		t.Fatalf("task dep exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "task dep requires a subcommand")
}

func TestInstallNeverOverwritesExistingAgentsEntrypoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# Project Agent Instructions\n\nKeep this.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--force", "init")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "skipped:", "  AGENTS.md")
	assertFileContainsAll(t, filepath.Join(root, "AGENTS.md"), "Keep this.")

	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.Files["AGENTS.md"]; ok {
		t.Fatal("AGENTS.md should not be recorded as a managed file")
	}
}

func TestUpgradeDecisions(t *testing.T) {
	root := t.TempDir()
	meta := metadata{
		Version: "0.0.1",
		Files: map[string]string{
			"AGENTS.md":                             hashBytes([]byte("old managed agents\n")),
			".agents/TASKS.md":                      hashBytes([]byte("old managed tasks\n")),
			".agents/PLANS.md":                      hashBytes(templateBytes(t, "workflow/PLANS.md")),
			".agents/RESEARCH.md":                   hashBytes([]byte("locally changed research\n")),
			".agents/DOCS.md":                       hashBytes([]byte("old managed docs\n")),
			".agents/.tasks/README.md":              hashBytes([]byte("old managed tasks readme\n")),
			".agents/.research/README.md":           hashBytes([]byte("old managed research readme\n")),
			".agents/.research/index.md":            hashBytes([]byte("old managed research index\n")),
			".agents/exec-plans/active/index.md":    hashBytes([]byte("old active plan index\n")),
			".agents/exec-plans/completed/index.md": hashBytes([]byte("old completed plan index\n")),
			"docs/adr/README.md":                    hashBytes([]byte("old managed adr\n")),
			".agents/skills/deslop/SKILL.md":        hashBytes([]byte("old managed skill\n")),
		},
	}
	for target := range meta.Files {
		content := "old managed\n"
		switch target {
		case "AGENTS.md":
			content = "old managed agents\n"
		case ".agents/TASKS.md":
			content = "old managed tasks\n"
		case ".agents/PLANS.md":
			content = string(templateBytes(t, "workflow/PLANS.md"))
		case ".agents/RESEARCH.md":
			content = "local edit that should conflict\n"
		case ".agents/DOCS.md":
			content = "old managed docs\n"
		case ".agents/.tasks/README.md":
			content = "old managed tasks readme\n"
		case ".agents/.research/README.md":
			content = "old managed research readme\n"
		case ".agents/.research/index.md":
			content = "old managed research index\n"
		case ".agents/exec-plans/active/index.md":
			content = "locally edited active plan index\n"
		case ".agents/exec-plans/completed/index.md":
			content = "locally edited completed plan index\n"
		case "docs/adr/README.md":
			content = "old managed adr\n"
		case ".agents/skills/deslop/SKILL.md":
			content = "old managed skill\n"
		}
		writeFile(t, filepath.Join(root, target), content)
	}
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.install(true); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		"skipped:",
		"  AGENTS.md",
		"  .agents/DOCS.md",
		"  .agents/PLANS.md",
		"conflicts:",
		"  .agents/RESEARCH.md",
	)

	after, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != templates.Version {
		t.Fatalf("metadata version = %q, want %q (version advances despite conflicts)", after.Version, templates.Version)
	}
	for _, target := range []string{
		".agents/.research/index.md",
		".agents/exec-plans/active/index.md",
		".agents/exec-plans/completed/index.md",
	} {
		if _, ok := after.Files[target]; ok {
			t.Fatalf("%s should not remain in metadata", target)
		}
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".research", "index.md"),
		"# Research Index",
		"This file is generated by `ahm index`.",
	)

	var forceOut strings.Builder
	forced := app{opts: options{root: root, force: true}, out: &forceOut}
	if err := forced.install(true); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, forceOut.String(), "updated:", "  .agents/RESEARCH.md")
	assertFileContainsAll(t, filepath.Join(root, "AGENTS.md"), "old managed agents")
	afterForce, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if afterForce.Version != templates.Version {
		t.Fatalf("forced metadata version = %q, want %q", afterForce.Version, templates.Version)
	}
}

func TestTaskStatusPreservesOptionalFrontMatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Preserve Metadata", "Pending", "depends_on: []\n"+
		"created: 2026-05-01\n"+
		"updated: 2026-05-02\n"+
		"parent: 000\n"+
		"external_ref: gh-123\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "completed", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"created: 2026-05-01",
		"updated: 2026-05-02",
		"parent: 000",
		"external_ref: gh-123",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing %q:\n%s", want, content)
		}
	}
}

func TestTaskDepUpdatePreservesOptionalFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending", "depends_on: []\n"+
		"created: 2026-05-01\n"+
		"updated: 2026-05-02\n"+
		"parent: 000\n"+
		"external_ref: gh-123\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependency", "Pending", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepUpdate([]string{"001", "002"}, true); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"depends_on: 002",
		"created: 2026-05-01",
		"updated: 2026-05-02",
		"parent: 000",
		"external_ref: gh-123",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing %q:\n%s", want, content)
		}
	}
}

func TestTaskStatusPreservesUnknownFrontMatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Unknown Fields", "Pending",
		"assignee: alice\n"+
			"due: 2026-06-01\n"+
			"tags: bug, urgent\n"+
			"ticket: JIRA-456\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "completed", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"assignee: alice",
		"due: 2026-06-01",
		"tags: bug, urgent",
		"ticket: JIRA-456",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing unknown field %q:\n%s", want, content)
		}
	}
}

func TestTaskDepUpdatePreservesUnknownFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending",
		"assignee: alice\n"+
			"due: 2026-06-01\n"+
			"tags: bug, urgent\n"+
			"ticket: JIRA-456\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependency", "Pending", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepUpdate([]string{"001", "002"}, true); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"assignee: alice",
		"due: 2026-06-01",
		"tags: bug, urgent",
		"ticket: JIRA-456",
		"depends_on: 002",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing %q:\n%s", want, content)
		}
	}
}

func TestFilterReadyAndBlockedTasks(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Completed", Priority: "P1"},
		{ID: "002", Status: "Pending", Priority: "P0", DependsOn: []string{"001"}},
		{ID: "003", Status: "Pending", Priority: "P2", DependsOn: []string{"004"}},
	}
	ready := filterTasks(tasks, "ready")
	if len(ready) != 1 || ready[0].ID != "002" {
		t.Fatalf("ready = %#v", ready)
	}
	blocked := filterTasks(tasks, "blocked")
	if len(blocked) != 1 || blocked[0].ID != "003" {
		t.Fatalf("blocked = %#v", blocked)
	}
}

func TestTaskListFiltersStatus(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Pending Task", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "002.md"), "002", "Completed Task", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "003.md"), "003", "Cancelled Task", "Cancelled", "")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskList("all", "completed"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got, "002 [Completed] P2 S Completed Task")
	assertNotContains(t, got, "001 [Pending]", "003 [Cancelled]")
}

func TestTaskNextShowsHighestPriorityReadyTask(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Done", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "P2 Ready", "Pending", "depends_on: 001\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "P1 Ready", "Pending", "P1", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Blocked", "Pending", "depends_on: 999\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskNext(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got, "003 [Pending] P1 S P1 Ready")
	assertNotContains(t, got, "002 [Pending]", "004 [Pending]")
}

func TestTaskDependencyTreeOutput(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Root", "Pending", "depends_on: 002, 999\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Middle", "Pending", "depends_on: 003\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Leaf", "Pending", "depends_on: 002\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepTree([]string{"001"}); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(),
		"001 [Pending] Root",
		"  002 [Pending] Middle",
		"    003 [Pending] Leaf",
		"      002 [Pending] Middle",
		"      cycle to 002",
		"  999 [missing]",
	)
}

func TestTaskDependencyCycleDetection(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Pending", DependsOn: []string{"002"}},
		{ID: "002", Status: "Pending", DependsOn: []string{"001"}},
		{ID: "003", Status: "Completed", DependsOn: []string{"004"}},
		{ID: "004", Status: "Pending", DependsOn: []string{"003"}},
		{ID: "005", Status: "Cancelled", DependsOn: []string{"006"}},
		{ID: "006", Status: "Pending", DependsOn: []string{"005"}},
	}
	cycles := taskDependencyCycles(tasks)
	if len(cycles) != 1 {
		t.Fatalf("cycles = %#v", cycles)
	}
	if got := strings.Join(cycles[0], " -> "); got != "001 -> 002 -> 001" {
		t.Fatalf("cycle = %q", got)
	}
}

func TestTaskDepCyclesCommand(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cycle A", "Pending", "depends_on: 002\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Cycle B", "Pending", "depends_on: 001\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "003.md"), "003", "Completed Cycle", "Completed", "depends_on: 004\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Ignored Active", "Pending", "depends_on: 003\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepCycles(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "001 -> 002 -> 001") {
		t.Fatalf("cycle output = %q", got)
	}
	if strings.Contains(got, "003") || strings.Contains(got, "004") {
		t.Fatalf("completed-task cycle should be ignored:\n%s", got)
	}
}

func TestRenderRootIndexGolden(t *testing.T) {
	index := renderRootIndex([]Task{
		{ID: "001", Title: "Done", Status: "Completed", Priority: "P1", Effort: "S", Labels: "type:task", ExecPlan: "plan-1", Bucket: "completed"},
		{ID: "002", Title: "Ready | Escape", Status: "Pending", Priority: "P0", Effort: "S", Labels: "type:task", ExecPlan: "-", Bucket: "active", DependsOn: []string{"001"}},
		{ID: "003", Title: "Blocked", Status: "Pending", Priority: "P2", Effort: "M", Labels: "type:task", ExecPlan: "-", Bucket: "active", DependsOn: []string{"004"}},
		{ID: "004", Title: "Open Needs Triage", Status: "Open", Priority: "P3", Effort: "L", Labels: "type:task", ExecPlan: "-", Bucket: "active"},
	})
	assertContainsAll(t, index,
		"# Task Index",
		"- Open: 1",
		"- Pending: 2",
		"- Completed: 1",
		"1. [002](active/002.md) - Ready | Escape (P0, S; type:task)",
		"| [003](active/003.md) | Blocked | Pending | P2 | M | type:task | - | 004 |",
		"| [004](active/004.md) | Open Needs Triage | Open | P3 | L | type:task | - | - |",
		"| [002](active/002.md) | Ready \\| Escape | Pending | P0 | S | type:task | - | 001 |",
		"| [001](completed/001.md) | Done | Completed | P1 | S | type:task | plan-1 | - |",
	)
}

func TestRenderBucketIndexGolden(t *testing.T) {
	tasks := []Task{
		{ID: "001", Title: "Active", Status: "Pending", Priority: "P1", Effort: "S", Labels: "type:task", ExecPlan: "-", Bucket: "active"},
		{ID: "002", Title: "Done", Status: "Completed", Priority: "P2", Effort: "M", Labels: "type:task", ExecPlan: "plan-2", Bucket: "completed"},
	}

	assertContainsAll(t, renderBucketIndex(tasks, "active"),
		"# Active Tasks",
		"| [001](001.md) | Active | Pending | P1 | S | type:task | - | - |",
	)
	assertContainsAll(t, renderBucketIndex(tasks, "completed"),
		"# Completed Tasks",
		"| [002](002.md) | Done | Completed | P2 | M | type:task | plan-2 | - |",
	)
	assertContainsAll(t, renderBucketIndex(tasks, "cancelled"),
		"# Cancelled Tasks",
		"None.",
	)
}

func TestRenderResearchIndexGolden(t *testing.T) {
	index := renderResearchIndex(map[string][]markdownDoc{
		"inbox": {
			{Link: "inbox/raw-note.md", Title: "Raw Capture"},
		},
		"investigations": {
			{Link: "investigations/session-review.md", Title: "Session Review"},
		},
		"sources": {},
		"topics": {
			{Link: "topics/index-policy.md", Title: "Index Policy"},
		},
		"archived": {},
	})
	assertContainsAll(t, index,
		"# Research Index",
		"This file is generated by `ahm index`. Do not edit it by hand.",
		"## Inbox",
		"- [Raw Capture](inbox/raw-note.md)",
		"## Investigations",
		"- [Session Review](investigations/session-review.md)",
		"## Sources",
		"No source notes yet.",
		"## Topics",
		"- [Index Policy](topics/index-policy.md)",
		"## Archived",
		"No archived research documents yet.",
	)
}

func TestRenderExecPlanIndexGolden(t *testing.T) {
	index := renderExecPlanIndex("Active ExecPlans", "No active ExecPlans yet.", map[string][]markdownDoc{
		"": {
			{Link: "001-build-indexes.md", Title: "Build Generated Indexes"},
		},
	})
	assertContainsAll(t, index,
		"# Active ExecPlans",
		"This file is generated by `ahm index`. Do not edit it by hand.",
		"- [Build Generated Indexes](001-build-indexes.md)",
	)

	empty := renderExecPlanIndex("Completed ExecPlans", "No completed ExecPlans yet.", map[string][]markdownDoc{"": {}})
	assertContainsAll(t, empty,
		"# Completed ExecPlans",
		"No completed ExecPlans yet.",
	)
}

func TestCollectMarkdownDocsUsesHeadingFallbackAndIgnoresIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "zeta.md"), "# Zeta Topic\n\nBody\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "alpha.md"), "No heading here.\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "index.md"), "# Ignore Me\n")

	docs, err := collectMarkdownDocs(root, ".agents/.research", []string{"topics"})
	if err != nil {
		t.Fatal(err)
	}
	got := docs["topics"]
	if len(got) != 2 {
		t.Fatalf("docs len = %d, want 2: %#v", len(got), got)
	}
	if got[0] != (markdownDoc{Link: "topics/alpha.md", Title: "alpha"}) {
		t.Fatalf("first doc = %#v", got[0])
	}
	if got[1] != (markdownDoc{Link: "topics/zeta.md", Title: "Zeta Topic"}) {
		t.Fatalf("second doc = %#v", got[1])
	}
}

func TestMainIndexRegeneratesResearchAndExecPlanIndexes(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	writeFile(t, filepath.Join(root, ".agents", ".research", "investigations", "cli-indexes.md"), "# CLI Indexes\n\nFindings.\n")
	writeFile(t, filepath.Join(root, ".agents", "exec-plans", "active", "generate-indexes.md"), "# Generate Indexes\n\nPlan.\n")
	writeFile(t, filepath.Join(root, ".agents", "exec-plans", "completed", "old-plan.md"), "# Old Plan\n\nDone.\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "index.md"), "stale\n")

	stdout, stderr, code = runCLI(t, "--root", root, "index")
	if code != 0 {
		t.Fatalf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".research", "index.md"),
		"# Research Index",
		"- [CLI Indexes](investigations/cli-indexes.md)",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", "exec-plans", "active", "index.md"),
		"# Active ExecPlans",
		"- [Generate Indexes](generate-indexes.md)",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", "exec-plans", "completed", "index.md"),
		"# Completed ExecPlans",
		"- [Old Plan](old-plan.md)",
	)
}

func TestMainTaskLifecycleAndDependencyIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "First Task", "--priority", "P1", "--effort", "M", "--description", "First body")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create first stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Second Task")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("create second stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "add", "002", "001")
	if code != 0 {
		t.Fatalf("dep add exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 depends_on: 001")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "depends_on: 001")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "blocked")
	if code != 0 {
		t.Fatalf("blocked exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending]")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "start", "001")
	if code != 0 {
		t.Fatalf("start exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> In Progress")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Fatalf("complete exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "ready")
	if code != 0 {
		t.Fatalf("ready exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending]")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "reopen", "001")
	if code != 0 {
		t.Fatalf("reopen exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "tree", "002")
	if code != 0 {
		t.Fatalf("dep tree exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending] Second Task", "  001 [Pending] First Task")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "remove", "002", "001")
	if code != 0 {
		t.Fatalf("dep remove exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 depends_on: -")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "depends_on: -")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "next")
	if code != 0 {
		t.Fatalf("next exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 [Pending] P1 M First Task")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "002")
	if code != 0 {
		t.Fatalf("cancel exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 -> Cancelled")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "002.md"), "status: Cancelled")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "list", "--status", "cancelled")
	if code != 0 {
		t.Fatalf("list --status exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Cancelled] P2 S Second Task")
	assertNotContains(t, stdout, "001 [Pending]")
}

func TestMainUpgradeIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "upgrade")
	if code != 0 {
		t.Fatalf("upgrade exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"skipped:",
		"  AGENTS.md",
		"  .agents/TASKS.md",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", "ahm.json"), `"version": "`+templates.Version+`"`)
}

func TestMainDependencyCyclesIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cycle A", "Pending", "depends_on: 002\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Cycle B", "Pending", "depends_on: 001\n")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "cycles")
	if code != 0 {
		t.Fatalf("cycles exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "001 -> 002 -> 001") {
		t.Fatalf("cycles stdout = %q", stdout)
	}
}

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout strings.Builder
	var stderr strings.Builder
	code := Main(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func runCLIFromDir(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	var stderr strings.Builder
	code := Main(args, &stdout, &stderr)
	if chErr := os.Chdir(origDir); chErr != nil {
		t.Errorf("failed to restore working directory: %v", chErr)
	}
	return stdout.String(), stderr.String(), code
}

func assertContainsAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func assertNotContains(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, item := range unwanted {
		if strings.Contains(got, item) {
			t.Fatalf("output unexpectedly contains %q:\n%s", item, got)
		}
	}
}

func assertFileContainsAll(t *testing.T, path string, wants ...string) {
	t.Helper()
	assertContainsAll(t, mustRead(t, path), wants...)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTaskFrontMatterReportsParseErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Block scalar in front matter should produce a parse error, not missing-field errors.
	content := "---\n" +
		"id: 001\n" +
		"title: Bad\n" +
		"status: Pending\n" +
		"priority: P1\n" +
		"effort: M\n" +
		"labels: type:bug\n" +
		"exec_plan: -\n" +
		"depends_on: -\n" +
		"description: |\n" +
		"  multi\n" +
		"  line\n" +
		"---\n" +
		"# Bad\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	report := &validationReport{}
	validateTaskFrontMatter(root, path, report)
	if len(report.Errors) == 0 {
		t.Fatal("expected at least one error, got none")
	}
	found := false
	for _, e := range report.Errors {
		if e.Code == "task_malformed" && strings.Contains(e.Message, "unsupported block scalar") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected task_malformed error with block scalar message, got: %v", report.Errors)
	}
	// Verify no missing-field errors (which would be misleading)
	for _, e := range report.Errors {
		if e.Code == "task_missing_field" {
			t.Fatalf("unexpected missing_field error when front matter is malformed: %v", e)
		}
	}
}

func TestFrontMatterValue(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"key: value", "value"},
		{"key: \"quoted\"", "quoted"},
		{"key:  spaced  ", "spaced"},
		{"no-colon", ""},
		{"key: value:more", "value:more"},
		{"key: \"quoted: with colon\"", "quoted: with colon"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := frontMatterValue(tt.line)
			if got != tt.want {
				t.Fatalf("frontMatterValue(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestDetectManagedRootFailsWithoutGitOrMetadata(t *testing.T) {
	root := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chErr := os.Chdir(origDir); chErr != nil {
			t.Errorf("failed to restore working directory: %v", chErr)
		}
	}()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	_, err = detectManagedRoot()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no .git or .agents/ahm.json found") {
		t.Fatalf("error should mention missing markers: %v", err)
	}
	if !strings.Contains(err.Error(), "--root") {
		t.Fatalf("error should mention --root: %v", err)
	}
	if !strings.Contains(err.Error(), "ahm init") {
		t.Fatalf("error should mention ahm init: %v", err)
	}
}

func TestDetectManagedRootSucceedsWithDotGit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chErr := os.Chdir(origDir); chErr != nil {
			t.Errorf("failed to restore working directory: %v", chErr)
		}
	}()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	got, err := detectManagedRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestDetectManagedRootSucceedsWithMetadata(t *testing.T) {
	root := t.TempDir()
	metaDir := filepath.Join(root, ".agents")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "ahm.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chErr := os.Chdir(origDir); chErr != nil {
			t.Errorf("failed to restore working directory: %v", chErr)
		}
	}()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	got, err := detectManagedRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestStrictCommandsFailOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()
	// Temp dir has no .git and no .agents/ahm.json

	for _, args := range [][]string{
		{"status"},
		{"doctor"},
		{"index"},
		{"task", "list"},
		{"task", "next"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, stderr, code := runCLIFromDir(t, root, args...)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1; stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, "no .git or .agents/ahm.json found") {
				t.Fatalf("stderr should mention no .git or .agents/ahm.json: %s", stderr)
			}
		})
	}
}

func TestInitSucceedsOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "created:", "  AGENTS.md")
}

func TestUpgradeSucceedsOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()

	// upgrade without prior install: lenient, acts like init
	stdout, stderr, code := runCLIFromDir(t, root, "upgrade")
	if code != 0 {
		t.Fatalf("upgrade exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "created:", "  AGENTS.md")

	// upgrade again: should skip unchanged files
	stdout, stderr, code = runCLIFromDir(t, root, "upgrade")
	if code != 0 {
		t.Fatalf("second upgrade exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "skipped:")
}

func TestStatusSucceedsAfterInitInCleanDir(t *testing.T) {
	root := t.TempDir()

	// init succeeds outside managed repo
	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "created:")

	// status now succeeds because .agents/ahm.json exists
	stdout, stderr, code = runCLIFromDir(t, root, "status")
	if code != 0 {
		t.Fatalf("status exit code = %d, stderr = %s", code, stderr)
	}
	// root in output is symlink-resolved; compare with evaluated path
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, evalRoot) {
		t.Fatalf("status output missing root %q:\n%s", evalRoot, stdout)
	}
	assertContainsAll(t, stdout, templates.Version)
}

func templateBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := fs.ReadFile(templates.FS, path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeTaskFile(t *testing.T, path string, id string, title string, status string, extraFrontMatter string) {
	t.Helper()
	writeTaskFileWithPriority(t, path, id, title, status, "P2", extraFrontMatter)
}

func writeTaskFileWithPriority(t *testing.T, path string, id string, title string, status string, priority string, extraFrontMatter string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"id: " + id + "\n" +
		"title: " + title + "\n" +
		"status: " + status + "\n" +
		"priority: " + priority + "\n" +
		"effort: S\n" +
		"labels: type:task\n" +
		"exec_plan: -\n" +
		extraFrontMatter +
		"---\n" +
		"# " + title + "\n\n" +
		"## Summary\n\nTODO.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
