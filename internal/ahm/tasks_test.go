package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontMatter(t *testing.T) {
	meta, body, err := parseFrontMatter("---\nid: 001\ntitle: Test Task\n---\n# Test Task\n")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if meta["id"] != "001" {
		t.Errorf("id = %q", meta["id"])
	}
	if meta["title"] != "Test Task" {
		t.Errorf("title = %q", meta["title"])
	}
	if !strings.Contains(body, "# Test Task") {
		t.Errorf("body = %q", body)
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
			name:  "quoted value preserves literal backslashes",
			input: "---\ntitle: \"C:\\\\Users\\\\test\"\n---\n# Task\n",
			want: map[string]string{
				"title": "C:\\\\Users\\\\test",
			},
		},
		{
			name:  "quoted value with escaped quote",
			input: "---\ntitle: \"say \\\"hello\\\"\"\n---\n# Task\n",
			want: map[string]string{
				"title": "say \"hello\"",
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
		{
			name:    "block list depends_on",
			input:   "---\ndepends_on:\n  - 001\n  - 002\n---\n# Task\n",
			wantErr: "unsupported block list syntax",
		},
		{
			name:    "block list labels",
			input:   "---\nlabels:\n  - type:bug\n  - area:tasks\n---\n# Task\n",
			wantErr: "unsupported block list syntax",
		},
		{
			name:    "block list standalone",
			input:   "---\n- item\n- other\n---\n# Task\n",
			wantErr: "unsupported block list syntax",
		},
		{
			name:  "quoted block scalar prefix round-trips",
			input: "---\ntitle: \"| block\"\n---\n# Task\n",
			want: map[string]string{
				"title": "| block",
			},
		},
		{
			name:  "unquoted value with internal quotes round-trips",
			input: "---\ntitle: say \"hello\"\n---\n# Task\n",
			want: map[string]string{
				"title": "say \"hello\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := parseFrontMatter(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Errorf("len(meta) = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("meta[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseFrontMatter_CRLF(t *testing.T) {
	// Even without going through readWorkflowFile, parseFrontMatter should
	// handle CRLF input due to its own normalization.
	meta, body, err := parseFrontMatter("---\r\nid: 001\r\ntitle: CRLF Task\r\n---\r\n# CRLF Task\r\n")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if meta["id"] != "001" {
		t.Errorf("id = %q", meta["id"])
	}
	if meta["title"] != "CRLF Task" {
		t.Errorf("title = %q", meta["title"])
	}
	if !strings.Contains(body, "# CRLF Task") {
		t.Errorf("body = %q", body)
	}
}

func TestParseTask_CRLF(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Write a task file with CRLF line endings.
	path := filepath.Join(root, ".ahm", "tasks", "active", "099.md")
	content := "---\r\n" +
		"id: 099\r\n" +
		"title: CRLF Task\r\n" +
		"status: Pending\r\n" +
		"priority: P2\r\n" +
		"effort: S\r\n" +
		"labels: type:test, area:workflow\r\n" +
		"exec_plan: -\r\n" +
		"depends_on: -\r\n" +
		"---\r\n" +
		"# CRLF Task\r\n" +
		"\r\n" +
		"## Summary\r\n" +
		"\r\n" +
		"Body with CRLF.\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	task, err := parseTask(path, "active")
	if err != nil {
		t.Errorf("parseTask: %v", err)
	}
	if task.ID != "099" {
		t.Errorf("task.ID = %q", task.ID)
	}
	if task.Title != "CRLF Task" {
		t.Errorf("task.Title = %q", task.Title)
	}
	if task.Status != "Pending" {
		t.Errorf("task.Status = %q", task.Status)
	}
	if !strings.Contains(task.Body, "Body with CRLF") {
		t.Errorf("task.Body = %q", task.Body)
	}
}

func TestHeadingTitle_CRLF(t *testing.T) {
	// headingTitle splits on "\n". CRLF lines have trailing \r, but
	// strings.HasPrefix should still match "# " because \r comes after.
	title := headingTitle("# CRLF Title\r\n\r\n## Section\r\n", "fallback")
	if title != "CRLF Title" {
		t.Errorf("headingTitle = %q, want %q", title, "CRLF Title")
	}
}

func TestStripHeading_CRLF(t *testing.T) {
	// stripHeading uses strings.TrimSpace per line, which strips \r.
	body := stripHeading("\r\n# CRLF Title\r\n\r\n## Section\r\n\r\nBody\r\n", "CRLF Title")
	if !strings.HasPrefix(body, "## Section") {
		t.Errorf("body = %q", body)
	}
}

func TestStripHeading(t *testing.T) {
	got := stripHeading("\n# Test Task\n\n## Summary\n\nBody\n", "Test Task")
	if !strings.HasPrefix(got, "## Summary") {
		t.Errorf("body = %q", got)
	}
}

func TestStripHeadingFormattedDuplicateH1(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		title string
	}{
		{
			name:  "inline code",
			body:  "\n# Fix `ahm task accept`\n\n## Summary\n\nBody\n",
			title: "Fix ahm task accept",
		},
		{
			name:  "strong emphasis",
			body:  "\n# Fix **formatted** title\n\n## Summary\n\nBody\n",
			title: "Fix formatted title",
		},
		{
			name:  "markdown link",
			body:  "\n# Fix [formatted title](https://example.test)\n\n## Summary\n\nBody\n",
			title: "Fix formatted title",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHeading(tt.body, tt.title)
			if !strings.HasPrefix(got, "## Summary") {
				t.Errorf("body = %q", got)
			}
			for _, line := range strings.Split(got, "\n") {
				if strings.HasPrefix(line, "# ") {
					t.Errorf("formatted duplicate H1 was preserved:\n%s", got)
				}
			}
		})
	}
}

func TestStripHeadingPreservesDifferentFormattedH1(t *testing.T) {
	got := stripHeading("\n# Different `Heading`\n\n## Summary\n\nBody\n", "Task Title")
	if !strings.HasPrefix(got, "# Different `Heading`") {
		t.Errorf("body = %q", got)
	}
}

func TestStripHeadingPreservesLiteralMarkdownTitleCharacters(t *testing.T) {
	got := stripHeading("\n# Use glob syntax\n\n## Summary\n\nBody\n", "Use * glob syntax")
	if !strings.HasPrefix(got, "# Use glob syntax") {
		t.Errorf("body = %q", got)
	}
}

func TestNextTaskID(t *testing.T) {
	got := nextTaskID([]Task{{ID: "001"}, {ID: "002a"}, {ID: "010"}}, t.TempDir())
	if got != "011" {
		t.Errorf("nextTaskID = %q", got)
	}
}

func TestNextTaskIDScansFilesystemForSkippedTasks(t *testing.T) {
	root := t.TempDir()
	// Create a valid task and a malformed one
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Valid", "Pending", "")
	// Malformed file with id 005 — should be picked up by filesystem scan
	if err := os.MkdirAll(filepath.Join(root, ".agents", ".tasks", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", ".tasks", "active", "005.md"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Only 001 is parsed; 005 is skipped due to parse error
	tasks, err := collectTasks(root)
	if err == nil {
		t.Error("expected error from malformed task")
	}
	if len(tasks) != 1 || tasks[0].ID != "001" {
		t.Errorf("expected only task 001, got %d tasks", len(tasks))
	}

	// nextTaskID should see 005 on disk and return 006
	got := nextTaskID(tasks, root)
	if got != "006" {
		t.Errorf("nextTaskID = %q, want %q", got, "006")
	}
}

func TestSplitTaskID(t *testing.T) {
	tests := []struct {
		id     string
		wantN  int
		wantS  string
		wantOk bool
	}{
		{id: "001", wantN: 1, wantS: "", wantOk: true},
		{id: "010", wantN: 10, wantS: "", wantOk: true},
		{id: "999999", wantN: 999999, wantS: "", wantOk: true},
		{id: "002a", wantN: 2, wantS: "a", wantOk: true},
		{id: "047b", wantN: 47, wantS: "b", wantOk: true},
		{id: "abc", wantN: 0, wantS: "abc", wantOk: false},
		{id: "", wantN: 0, wantS: "", wantOk: false},
		{id: "12", wantN: 12, wantS: "", wantOk: true},
	}
	for _, tt := range tests {
		n, s, ok := splitTaskID(tt.id)
		if n != tt.wantN || s != tt.wantS || ok != tt.wantOk {
			t.Errorf("splitTaskID(%q) = (%d, %q, %v), want (%d, %q, %v)",
				tt.id, n, s, ok, tt.wantN, tt.wantS, tt.wantOk)
		}
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
			t.Error(err)
		}
		if task.ID != "001" {
			t.Errorf("id = %q", task.ID)
		}
	})

	t.Run("exact match second form", func(t *testing.T) {
		task, err := a.resolveTask("010")
		if err != nil {
			t.Error(err)
		}
		if task.ID != "010" {
			t.Errorf("id = %q", task.ID)
		}
	})

	t.Run("numeric prefix matches zero-padded", func(t *testing.T) {
		task, err := a.resolveTask("1")
		if err != nil {
			t.Error(err)
		}
		if task.ID != "001" {
			t.Errorf("id = %q", task.ID)
		}
	})

	t.Run("numeric prefix with leading zero", func(t *testing.T) {
		task, err := a.resolveTask("01")
		if err != nil {
			t.Error(err)
		}
		if task.ID != "001" {
			t.Errorf("id = %q", task.ID)
		}
	})

	t.Run("numeric prefix matches 010", func(t *testing.T) {
		task, err := a.resolveTask("10")
		if err != nil {
			t.Error(err)
		}
		if task.ID != "010" {
			t.Errorf("id = %q", task.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := a.resolveTask("999")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("pattern zero not found", func(t *testing.T) {
		// "0" would match all tasks via string prefix, but only matches
		// numerically if a task has numeric part 0. None do, so expect not found.
		_, err := a.resolveTask("0")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not found, got: %v", err)
		}
	})

	t.Run("exact numeric match preferred over prefix match", func(t *testing.T) {
		// Add a task with a suffix that also has num=1
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001a.md"), "001a", "Task One A", "Pending", "")
		a.invalidateTasks()
		task, err := a.resolveTask("1")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if task.ID != "001" {
			t.Errorf("expected 001, got %s", task.ID)
		}
	})
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
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), `unsupported task priority "Urgent"`) {
		t.Errorf("error = %q", err)
	}
}

func TestResolveTaskFromTasks(t *testing.T) {
	tasks := []Task{
		{ID: "001", Title: "Alpha", Status: "Pending"},
		{ID: "002", Title: "Beta", Status: "Pending"},
	}
	t.Run("exact match", func(t *testing.T) {
		task, err := resolveTaskFromTasks("001", tasks)
		if err != nil {
			t.Error(err)
		}
		if task.ID != "001" {
			t.Errorf("expected 001, got %s", task.ID)
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, err := resolveTaskFromTasks("999", tasks)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not-found error, got: %v", err)
		}
	})
}

func TestRenderTaskCanonicalOrder(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "non-canonical input order",
			input: "---\n" +
				"effort: L\n" +
				"priority: P1\n" +
				"status: Pending\n" +
				"labels: type:test, area:tasks\n" +
				"title: Non-canonical Order\n" +
				"id: 099\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Non-canonical Order\n\nBody.\n",
			want: "---\n" +
				"id: 099\n" +
				"title: Non-canonical Order\n" +
				"status: Pending\n" +
				"priority: P1\n" +
				"effort: L\n" +
				"labels: type:test, area:tasks\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Non-canonical Order\n\nBody.\n\n",
		},
		{
			name: "empty exec_plan normalizes to dash",
			input: "---\n" +
				"id: 102\n" +
				"title: Empty ExecPlan\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:test\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Empty ExecPlan\n\nBody.\n",
			want: "---\n" +
				"id: 102\n" +
				"title: Empty ExecPlan\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:test\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Empty ExecPlan\n\nBody.\n\n",
		},
		{
			name: "optional fields omitted when empty",
			input: "---\n" +
				"id: 100\n" +
				"title: No Optional Fields\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:test\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# No Optional Fields\n\nBody.\n",
			want: "---\n" +
				"id: 100\n" +
				"title: No Optional Fields\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:test\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# No Optional Fields\n\nBody.\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := parseTaskFromData([]byte(tt.input), "unused.md", "active")
			if err != nil {
				t.Errorf("parseTaskFromData: %v", err)
			}
			got := renderTask(task)
			if got != tt.want {
				t.Errorf("renderTask output mismatch\ngot:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestRenderTaskExtraFieldsSorted(t *testing.T) {
	input := "---\n" +
		"id: 101\n" +
		"title: Extra Fields Roundtrip\n" +
		"status: Pending\n" +
		"priority: P2\n" +
		"effort: S\n" +
		"labels: type:test\n" +
		"exec_plan: -\n" +
		"depends_on: -\n" +
		"zeta_field: last\n" +
		"alpha_field: first\n" +
		"beta_field: middle\n" +
		"---\n" +
		"# Extra Fields Roundtrip\n\nBody.\n"

	want := "---\n" +
		"id: 101\n" +
		"title: Extra Fields Roundtrip\n" +
		"status: Pending\n" +
		"priority: P2\n" +
		"effort: S\n" +
		"labels: type:test\n" +
		"exec_plan: -\n" +
		"depends_on: -\n" +
		"alpha_field: first\n" +
		"beta_field: middle\n" +
		"zeta_field: last\n" +
		"---\n" +
		"# Extra Fields Roundtrip\n\nBody.\n\n"

	task, err := parseTaskFromData([]byte(input), "unused.md", "active")
	if err != nil {
		t.Errorf("parseTaskFromData: %v", err)
	}
	if len(task.Extra) != 3 {
		t.Errorf("Expected 3 extra fields, got %d: %v", len(task.Extra), task.Extra)
	}

	got := renderTask(task)
	if got != want {
		t.Errorf("renderTask with extra fields mismatch\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestRenderFrontMatterScalar(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "Plain Value", "Plain Value"},
		{"colon", "type:bug", "type:bug"},
		{"empty", "", "\"\""},
		{"whitespace trimmed", "  padded  ", "padded"},
		{"leading hash", "#not a comment", "\"#not a comment\""},
		{"block scalar pipe", "| block", "\"| block\""},
		{"block scalar gt", "> block", "\"> block\""},
		{"block list", "- list item", "\"- list item\""},
		{"dash alone", "-", "-"},
		{"leading quote", "\"quoted", "\"\\\"quoted\""},
		{"internal quotes plain", "say \"hello\"", "say \"hello\""},
		{"backslash plain", "C:\\Users\\test", "C:\\Users\\test"},
		{"newline collapsed", "line1\nline2", "line1 line2"},
		{"crlf collapsed", "line1\r\nline2", "line1 line2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderFrontMatterScalar(tt.value)
			if got != tt.want {
				t.Errorf("renderFrontMatterScalar(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestRenderTaskRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]string
	}{
		{
			name: "block scalar prefix in title",
			fields: map[string]string{
				"id":     "100",
				"title":  "| block scalar",
				"labels": "type:test",
			},
		},
		{
			name: "block list prefix in title",
			fields: map[string]string{
				"id":     "101",
				"title":  "- list like",
				"labels": "type:test",
			},
		},
		{
			name: "internal quotes in title",
			fields: map[string]string{
				"id":     "102",
				"title":  "Important \"task\"",
				"labels": "type:test",
			},
		},
		{
			name: "quoted title",
			fields: map[string]string{
				"id":     "106",
				"title":  `"Important" task`,
				"labels": "type:test",
			},
		},
		{
			name: "title of only double quotes",
			fields: map[string]string{
				"id":     "107",
				"title":  `""`,
				"labels": "type:test",
			},
		},
		{
			name: "title with backslash and quote",
			fields: map[string]string{
				"id":     "108",
				"title":  `"\"`,
				"labels": "type:test",
			},
		},
		{
			name: "backslash in title",
			fields: map[string]string{
				"id":     "105",
				"title":  `C:\Users\test`,
				"labels": "type:test",
			},
		},
		{
			name: "colon in label",
			fields: map[string]string{
				"id":     "103",
				"title":  "Task",
				"labels": "type:test, area:workflow",
			},
		},
		{
			name: "extra field with special content",
			fields: map[string]string{
				"id":          "104",
				"title":       "Task",
				"labels":      "type:test",
				"extra_note":  "# not a comment",
				"extra_block": "| not a block",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := map[string]string{
				"status":     "Pending",
				"priority":   "P2",
				"effort":     "S",
				"exec_plan":  "-",
				"depends_on": "-",
			}
			for k, v := range tt.fields {
				meta[k] = v
			}
			input := renderTask(Task{
				ID:          meta["id"],
				Title:       meta["title"],
				Status:      meta["status"],
				Priority:    meta["priority"],
				Effort:      meta["effort"],
				Labels:      meta["labels"],
				ExecPlan:    meta["exec_plan"],
				DependsOn:   nil,
				Created:     "",
				Updated:     "",
				Parent:      "",
				ExternalRef: "",
				Extra:       metaExtra(meta),
				Body:        "Body.",
			})
			task, err := parseTaskFromData([]byte(input), "unused.md", "active")
			if err != nil {
				t.Fatalf("parseTaskFromData: %v", err)
			}
			if task.Title != meta["title"] {
				t.Errorf("title round-trip = %q, want %q", task.Title, meta["title"])
			}
			if task.Labels != meta["labels"] {
				t.Errorf("labels round-trip = %q, want %q", task.Labels, meta["labels"])
			}
			for k, v := range tt.fields {
				switch k {
				case "id", "title", "status", "priority", "effort", "labels", "exec_plan", "depends_on":
					continue
				}
				if task.Extra[k] != v {
					t.Errorf("extra %q round-trip = %q, want %q", k, task.Extra[k], v)
				}
			}
		})
	}
}
