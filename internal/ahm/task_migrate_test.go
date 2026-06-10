package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFrontMatter_CRLF(t *testing.T) {
	raw, body, ok := splitFrontMatter("---\r\nid: 001\r\n---\r\n# Body\r\n")
	if !ok {
		t.Fatal("splitFrontMatter returned false for CRLF input")
	}
	if !strings.Contains(raw, "id: 001") {
		t.Fatalf("raw = %q", raw)
	}
	if !strings.Contains(body, "# Body") {
		t.Fatalf("body = %q", body)
	}
}

func TestMigrateTaskFrontMatter_CRLF(t *testing.T) {
	// migrateTaskFrontMatter calls splitFrontMatter which normalizes CRLF.
	input := "---\r\n" +
		"id: 099\r\n" +
		"title: Legacy Task\r\n" +
		"status: Pending\r\n" +
		"priority: -\r\n" +
		"effort: XL (split)\r\n" +
		"exec_plan: -\r\n" +
		"depends_on: -\r\n" +
		"---\r\n" +
		"# Legacy Task\r\n"

	result, changes := migrateTaskFrontMatter(input)
	if len(changes) == 0 {
		t.Fatal("expected migrations for legacy CRLF task")
	}
	// The result should only use LF line endings.
	if strings.Contains(result, "\r\n") {
		t.Fatalf("migration output contains CRLF: %q", result)
	}
	if !strings.Contains(changes[0], "add labels") {
		t.Fatalf("first change = %q, want 'add labels'", changes[0])
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

func TestMigrateTaskFrontMatter_MultipleInsertAndNormalize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changes []string
	}{
		{
			name: "missing labels, normalize priority and effort, normalize depends_on",
			input: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: -\n" +
				"effort: XL (split)\n" +
				"exec_plan: -\n" +
				"depends_on: 010 (some note)\n" +
				"---\n" +
				"# Test\n",
			want: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: P3\n" +
				"effort: XL\n" +
				"labels: type:task, area:unknown\n" +
				"exec_plan: -\n" +
				"depends_on: 010\n" +
				"---\n" +
				"# Test\n",
			changes: []string{
				"add labels",
				"set priority placeholder to P3",
				"normalize effort to XL",
				"normalize depends_on",
			},
		},
		{
			name: "all fields already present and already normalized",
			input: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:task, area:cli\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Test\n",
			want: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:task, area:cli\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Test\n",
			changes: nil,
		},
		{
			name: "only labels missing, rest already normalized",
			input: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Test\n",
			want: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:task, area:unknown\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Test\n",
			changes: []string{"add labels"},
		},
		{
			name: "effort placeholder - set to M, priority already valid",
			input: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: P1\n" +
				"effort: -\n" +
				"labels: type:bug, area:tasks\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Test\n",
			want: "---\n" +
				"id: 001\n" +
				"title: Test\n" +
				"status: Pending\n" +
				"priority: P1\n" +
				"effort: M\n" +
				"labels: type:bug, area:tasks\n" +
				"exec_plan: -\n" +
				"depends_on: -\n" +
				"---\n" +
				"# Test\n",
			changes: []string{"set effort placeholder to M"},
		},
		{
			name: "depends_on follows-form and normalized priority",
			input: "---\n" +
				"id: 005\n" +
				"title: Follow-up\n" +
				"status: Pending\n" +
				"priority: P2 (Medium)\n" +
				"effort: S\n" +
				"labels: type:task\n" +
				"exec_plan: -\n" +
				"depends_on: Follows 004\n" +
				"---\n" +
				"# Follow-up\n",
			want: "---\n" +
				"id: 005\n" +
				"title: Follow-up\n" +
				"status: Pending\n" +
				"priority: P2\n" +
				"effort: S\n" +
				"labels: type:task\n" +
				"exec_plan: -\n" +
				"depends_on: 004\n" +
				"---\n" +
				"# Follow-up\n",
			changes: []string{
				"normalize priority to P2",
				"normalize depends_on",
			},
		},
		{
			name:    "block list depends_on rejected",
			input:   "---\nid: 001\ntitle: Test\nstatus: Pending\npriority: P2\neffort: S\nlabels: type:bug, area:tasks\nexec_plan: -\ndepends_on:\n  - 001\n  - 002\n---\n# Test\n",
			want:    "---\nid: 001\ntitle: Test\nstatus: Pending\npriority: P2\neffort: S\nlabels: type:bug, area:tasks\nexec_plan: -\ndepends_on:\n  - 001\n  - 002\n---\n# Test\n",
			changes: []string{"unsupported block list syntax in front matter; fix manually"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changes := migrateTaskFrontMatter(tt.input)
			if got != tt.want {
				t.Fatalf("migrateTaskFrontMatter() output =\n%s\n\nwant =\n%s", got, tt.want)
			}
			if len(changes) == 0 && len(tt.changes) == 0 {
				return
			}
			if len(changes) != len(tt.changes) {
				t.Fatalf("changes = %v (len=%d), want %v (len=%d)", changes, len(changes), tt.changes, len(tt.changes))
			}
			for i, c := range changes {
				if c != tt.changes[i] {
					t.Fatalf("changes[%d] = %q, want %q", i, c, tt.changes[i])
				}
			}
		})
	}
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
