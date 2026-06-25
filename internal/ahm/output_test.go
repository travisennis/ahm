package ahm

import (
	"strings"
	"testing"
)

func TestEmitText_MapStringAny(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "flat map with mixed types",
			value: map[string]any{
				"root":    "/path",
				"enabled": true,
				"count":   42,
			},
			want: "count: 42\nenabled: true\nroot: /path\n",
		},
		{
			name: "map with nested map",
			value: map[string]any{
				"tasks": map[string]any{
					"total":   5,
					"pending": 2,
				},
				"root": "/path",
			},
			want: "root: /path\ntasks:\n  pending: 2\n  total: 5\n",
		},
		{
			name: "map with nested slice of maps",
			value: map[string]any{
				"errors": []map[string]any{
					{"code": "ERR1", "message": "first error"},
				},
				"root": "/path",
			},
			want: "errors:\n  - code: ERR1\n    message: first error\nroot: /path\n",
		},
		{
			name: "nil value",
			value: map[string]any{
				"installed": nil,
				"root":      "/path",
			},
			want: "installed: none\nroot: /path\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			a := app{out: &out}
			if err := a.emit(tt.value); err != nil {
				t.Error(err)
			}
			got := out.String()
			if got != tt.want {
				t.Errorf("emitText output mismatch:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestEmitText_MapStringSliceString(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "install result with sections",
			value: map[string][]string{
				"created":   {"AGENTS.md", ".agents/TASKS.md"},
				"updated":   {},
				"skipped":   {".agents/DOCS.md"},
				"conflicts": {},
			},
			want: "created:\n  AGENTS.md\n  .agents/TASKS.md\nskipped:\n  .agents/DOCS.md\n",
		},
		{
			name: "empty map",
			value: map[string][]string{
				"created": {},
				"updated": {},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			a := app{out: &out}
			if err := a.emit(tt.value); err != nil {
				t.Error(err)
			}
			got := out.String()
			if got != tt.want {
				t.Errorf("emitText output mismatch:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestEmitText_DryRunMap(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "task create dry-run",
			value: map[string]any{"create": ".agents/.tasks/active/001.md", "id": "001"},
			want:  "create: .agents/.tasks/active/001.md\nid: 001\n",
		},
		{
			name:  "task status dry-run",
			value: map[string]any{"move": ".agents/.tasks/completed/001.md", "status": "Completed"},
			want:  "move: .agents/.tasks/completed/001.md\nstatus: Completed\n",
		},
		{
			name:  "dep update dry-run",
			value: map[string]any{"task": "001", "depends_on": []string{"002", "003"}},
			want:  "depends_on:\n  - 002\n  - 003\ntask: 001\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			a := app{out: &out}
			if err := a.emit(tt.value); err != nil {
				t.Error(err)
			}
			got := out.String()
			if got != tt.want {
				t.Errorf("emitText output mismatch:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestEmitText_NestedTypedMapInAny(t *testing.T) {
	// Verify that typed maps nested inside map[string]any render as
	// YAML-like text, not Go's raw map[...] format.
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "nested map[string]int",
			value: map[string]any{
				"root":  "/path",
				"tasks": map[string]int{"completed": 2, "pending": 5},
			},
			want: "root: /path\ntasks:\n  completed: 2\n  pending: 5\n",
		},
		{
			name: "nested map[string]string",
			value: map[string]any{
				"root": "/path",
				"meta": map[string]string{"version": "1.0", "scope": "local"},
			},
			want: "meta:\n  scope: local\n  version: 1.0\nroot: /path\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			a := app{out: &out}
			if err := a.emit(tt.value); err != nil {
				t.Error(err)
			}
			got := out.String()
			if got != tt.want {
				t.Errorf("emitText output mismatch:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestEmitJSON_StatusMap(t *testing.T) {
	// Verify --json mode still produces valid JSON for status/doctor maps.
	var out strings.Builder
	a := app{opts: options{json: true}, out: &out}
	value := map[string]any{
		"root":    "/path",
		"enabled": true,
	}
	if err := a.emit(value); err != nil {
		t.Error(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(strings.TrimSpace(got), "}") {
		t.Errorf("JSON output should be a JSON object, got: %q", got)
	}
	if !strings.Contains(got, `"root": "/path"`) {
		t.Errorf("JSON output missing expected field: %q", got)
	}
	if !strings.Contains(got, `"enabled": true`) {
		t.Errorf("JSON output missing expected field: %q", got)
	}
}

func TestEmitPlain_ProducesCompactJSON(t *testing.T) {
	var out strings.Builder
	a := app{opts: options{plain: true}, out: &out}
	value := map[string]any{
		"root": "/path",
	}
	if err := a.emit(value); err != nil {
		t.Error(err)
	}
	got := out.String()
	// Compact JSON should be on one line (no newlines before the final one).
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("plain output should be a single line, got %d lines: %q", len(lines), got)
	}
	if !strings.Contains(got, `{"root":"/path"}`) {
		t.Errorf("plain output missing expected content: %q", got)
	}
}

func TestEmitText_NilValue(t *testing.T) {
	var out strings.Builder
	a := app{out: &out}
	if err := a.emit(nil); err != nil {
		t.Error(err)
	}
	got := out.String()
	if got != "none\n" {
		t.Errorf("emit(nil) = %q, want %q", got, "none\n")
	}
}

func TestEmitJSON_NilValue(t *testing.T) {
	var out strings.Builder
	a := app{opts: options{json: true}, out: &out}
	if err := a.emit(nil); err != nil {
		t.Error(err)
	}
	got := out.String()
	if got != "null\n" {
		t.Errorf("emitJSON(nil) = %q, want %q", got, "null\n")
	}
}
